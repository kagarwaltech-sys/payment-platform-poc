package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"payment-platform/backend/internal/payment"
	"payment-platform/backend/internal/processor/mock"
)

var migrationOnce sync.Once
var migrationErr error

func integrationService(t *testing.T) (*Service, *mock.Processor) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	s, err := New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Pool.Close)
	migrationOnce.Do(func() {
		_, source, _, _ := runtime.Caller(0)
		migration, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "migrations", "001_initial.sql"))
		if err == nil {
			_, err = s.Pool.Exec(context.Background(), string(migration))
		}
		migrationErr = err
	})
	if migrationErr != nil {
		t.Fatal(migrationErr)
	}
	if _, err = s.Pool.Exec(context.Background(), `TRUNCATE payments, idempotency_records, ledger_entries, ledger_journals, ledger_accounts, payment_captures CASCADE`); err != nil {
		t.Fatal(err)
	}
	p := &mock.Processor{}
	return &Service{Store: s, Processor: p}, p
}
func createAuthorized(t *testing.T, s *Service, key string, amount int64) payment.Payment {
	t.Helper()
	p, _, err := s.Create(context.Background(), key, CreateRequest{Amount: amount, Currency: "USD", CaptureMethod: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err = s.Authorize(context.Background(), p.ID, key+"-auth")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPaymentCaptureIntegration(t *testing.T) {
	s, processor := integrationService(t)
	ctx := context.Background()
	p, code, err := s.Create(ctx, "create", CreateRequest{Amount: 10000, Currency: "USD", CaptureMethod: "manual", Reference: "order-1"})
	if err != nil || code != 201 || p.Status != payment.Created {
		t.Fatalf("create: %#v %d %v", p, code, err)
	}
	if _, _, _, err = s.Capture(ctx, p.ID, "before-auth", CaptureRequest{Amount: 1}); !errors.Is(err, payment.ErrInvalidTransition) {
		t.Fatalf("expected pre-authorization capture rejection: %v", err)
	}
	if _, _, err = s.Authorize(ctx, p.ID, "bad-auth"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Authorize(ctx, p.ID, "again-auth"); !errors.Is(err, payment.ErrInvalidTransition) {
		t.Fatalf("expected invalid authorization: %v", err)
	}
	p, _, _, err = s.Capture(ctx, p.ID, "cap-1", CaptureRequest{Amount: 4000})
	if err != nil || p.Status != payment.PartiallyCaptured || p.CapturedAmount != 4000 {
		t.Fatalf("partial capture: %#v %v", p, err)
	}
	p, _, _, err = s.Capture(ctx, p.ID, "cap-2", CaptureRequest{Amount: 6000})
	if err != nil || p.Status != payment.Captured || p.CapturedAmount != 10000 {
		t.Fatalf("full capture: %#v %v", p, err)
	}
	if _, _, _, err = s.Capture(ctx, p.ID, "cap-over", CaptureRequest{Amount: 1}); !errors.Is(err, payment.ErrInvalidTransition) {
		t.Fatalf("expected completed capture rejection: %v", err)
	}
	_, captures := processor.Counts()
	if captures != 2 {
		t.Fatalf("capture calls=%d", captures)
	}
	var journals int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM ledger_journals`).Scan(&journals); err != nil || journals != 2 {
		t.Fatalf("journals=%d %v", journals, err)
	}
	var imbalance int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM (SELECT journal_id, sum(CASE direction WHEN 'DEBIT' THEN amount ELSE -amount END) n FROM ledger_entries GROUP BY journal_id HAVING sum(CASE direction WHEN 'DEBIT' THEN amount ELSE -amount END) <> 0) x`).Scan(&imbalance); err != nil || imbalance != 0 {
		t.Fatalf("imbalance=%d %v", imbalance, err)
	}
}

func TestCaptureIdempotencyAndConcurrency(t *testing.T) {
	s, processor := integrationService(t)
	ctx := context.Background()
	p := createAuthorized(t, s, "p", 100)
	first, ref, _, err := s.Capture(ctx, p.ID, "same", CaptureRequest{Amount: 40})
	if err != nil {
		t.Fatal(err)
	}
	again, againRef, _, err := s.Capture(ctx, p.ID, "same", CaptureRequest{Amount: 40})
	if err != nil || again.CapturedAmount != first.CapturedAmount || againRef != ref {
		t.Fatalf("replay: %#v %s %v", again, againRef, err)
	}
	if _, _, _, err = s.Capture(ctx, p.ID, "same", CaptureRequest{Amount: 30}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict: %v", err)
	}
	_, n := processor.Counts()
	if n != 1 {
		t.Fatalf("replay called PSP %d times", n)
	}
	p = createAuthorized(t, s, "same-concurrent", 100)
	var sameWG sync.WaitGroup
	for range 2 {
		sameWG.Add(1)
		go func() {
			defer sameWG.Done()
			_, _, _, _ = s.Capture(ctx, p.ID, "same-concurrent-cap", CaptureRequest{Amount: 10})
		}()
	}
	sameWG.Wait()
	_, n = processor.Counts()
	if n != 2 {
		t.Fatalf("concurrent same key called PSP %d times", n)
	}
	p = createAuthorized(t, s, "concurrent", 100)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, k := range []string{"a", "b"} {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			_, _, _, e := s.Capture(ctx, p.ID, k, CaptureRequest{Amount: 60})
			results <- e
		}(k)
	}
	wg.Wait()
	close(results)
	ok := 0
	for e := range results {
		if e == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("concurrent successful captures=%d", ok)
	}
	got, err := s.Store.Get(ctx, p.ID)
	if err != nil || got.CapturedAmount != 60 {
		t.Fatalf("over-capture protection: %#v %v", got, err)
	}
}

func TestCaptureRollbackWhenLedgerWriteFails(t *testing.T) {
	s, _ := integrationService(t)
	ctx := context.Background()
	p := createAuthorized(t, s, "rollback", 100)
	if _, err := s.Store.Pool.Exec(ctx, `ALTER TABLE ledger_entries ADD CONSTRAINT test_ledger_failure CHECK (false) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.Store.Pool.Exec(context.Background(), `ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS test_ledger_failure`)
	})
	if _, _, _, err := s.Capture(ctx, p.ID, "rollback-cap", CaptureRequest{Amount: 10}); err == nil {
		t.Fatal("expected local ledger failure")
	}
	got, err := s.Store.Get(ctx, p.ID)
	if err != nil || got.CapturedAmount != 0 || got.Status != payment.Authorized {
		t.Fatalf("payment was not rolled back: %#v %v", got, err)
	}
	var count int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM payment_captures WHERE payment_id=$1`, p.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("capture was not rolled back: %d %v", count, err)
	}
}
