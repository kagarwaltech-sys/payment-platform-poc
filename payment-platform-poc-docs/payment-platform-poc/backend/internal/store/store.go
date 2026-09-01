package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"payment-platform/backend/internal/payment"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrIdempotencyConflict = errors.New("idempotency key reused with a different request")
var ErrInProgress = errors.New("idempotency operation is already in progress")
var ErrPartialCaptureUnsupported = errors.New("processor does not support partial capture")

type Store struct{ Pool *pgxpool.Pool }

func New(ctx context.Context, dsn string) (*Store, error) {
	p, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Store{p}, p.Ping(ctx)
}
func Hash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type IdempotencyResult struct {
	Completed bool
	Code      int
	Body      json.RawMessage
}

// Claim persists ownership before any PSP call. A second concurrent caller observes IN_PROGRESS and never calls the processor.
func (s *Store) Claim(ctx context.Context, key, operation, hash string) (IdempotencyResult, error) {
	tag, err := s.Pool.Exec(ctx, `INSERT INTO idempotency_records(idempotency_key,operation,request_hash,status) VALUES($1,$2,$3,'IN_PROGRESS') ON CONFLICT DO NOTHING`, key, operation, hash)
	if err != nil {
		return IdempotencyResult{}, err
	}
	owner := tag.RowsAffected() == 1
	var gotHash, status string
	var code *int
	var body []byte
	err = s.Pool.QueryRow(ctx, `SELECT request_hash,status,response_code,response_body FROM idempotency_records WHERE idempotency_key=$1 AND operation=$2`, key, operation).Scan(&gotHash, &status, &code, &body)
	if err != nil {
		return IdempotencyResult{}, err
	}
	if gotHash != hash {
		return IdempotencyResult{}, ErrIdempotencyConflict
	}
	if status == "COMPLETED" {
		return IdempotencyResult{true, *code, body}, nil
	}
	if owner {
		return IdempotencyResult{}, nil
	}
	return IdempotencyResult{}, ErrInProgress
}

func (s *Store) Complete(ctx context.Context, tx pgx.Tx, key, operation string, resourceID string, code int, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE idempotency_records SET status='COMPLETED',resource_id=NULLIF($3,'')::uuid,response_code=$4,response_body=$5,updated_at=now() WHERE idempotency_key=$1 AND operation=$2 AND status='IN_PROGRESS'`, key, operation, resourceID, code, b)
	return err
}
func (s *Store) Fail(ctx context.Context, key, operation string) {
	_, _ = s.Pool.Exec(ctx, `UPDATE idempotency_records SET status='FAILED',updated_at=now() WHERE idempotency_key=$1 AND operation=$2 AND status='IN_PROGRESS'`, key, operation)
}

func scanPayment(row pgx.Row) (payment.Payment, error) {
	var p payment.Payment
	err := row.Scan(&p.ID, &p.Amount, &p.Currency, &p.AuthorizedAmount, &p.CapturedAmount, &p.RefundedAmount, &p.Status, &p.CaptureMethod, &p.Reference, &p.Processor, &p.ProcessorPaymentID)
	return p, err
}

const paymentColumns = `id::text,amount,currency,authorized_amount,captured_amount,refunded_amount,status,capture_method,COALESCE(reference,''),COALESCE(processor,''),COALESCE(processor_payment_id,'')`

func (s *Store) Get(ctx context.Context, id string) (payment.Payment, error) {
	return scanPayment(s.Pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id=$1`, id))
}
func getForUpdate(ctx context.Context, tx pgx.Tx, id string) (payment.Payment, error) {
	return scanPayment(tx.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id=$1 FOR UPDATE`, id))
}

type Service struct {
	Store     *Store
	Processor payment.Processor
}
type CreateRequest struct {
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	CaptureMethod string `json:"capture_method"`
	Reference     string `json:"reference"`
}
type CaptureRequest struct {
	Amount int64 `json:"amount"`
}
type RefundRequest struct {
	Amount int64 `json:"amount"`
}

type CaptureActivity struct {
	ID                 string    `json:"id"`
	Amount             int64     `json:"amount"`
	ProcessorCaptureID string    `json:"processor_capture_id"`
	CreatedAt          time.Time `json:"created_at"`
}

type RefundActivity struct {
	ID                string    `json:"id"`
	Amount            int64     `json:"amount"`
	ProcessorRefundID string    `json:"processor_refund_id"`
	CreatedAt         time.Time `json:"created_at"`
}

type JournalActivity struct {
	ID            string    `json:"id"`
	EventType     string    `json:"event_type"`
	ReferenceType string    `json:"reference_type"`
	ReferenceID   string    `json:"reference_id"`
	Currency      string    `json:"currency"`
	Debit         int64     `json:"debit"`
	Credit        int64     `json:"credit"`
	CreatedAt     time.Time `json:"created_at"`
}

type PaymentActivity struct {
	Captures []CaptureActivity `json:"captures"`
	Refunds  []RefundActivity  `json:"refunds"`
	Journals []JournalActivity `json:"journals"`
}

func (s *Store) Activity(ctx context.Context, id string) (PaymentActivity, error) {
	var activity PaymentActivity
	rows, err := s.Pool.Query(ctx, `SELECT id::text,amount,processor_capture_id,created_at FROM payment_captures WHERE payment_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return activity, err
	}
	defer rows.Close()
	for rows.Next() {
		var item CaptureActivity
		if err := rows.Scan(&item.ID, &item.Amount, &item.ProcessorCaptureID, &item.CreatedAt); err != nil {
			return activity, err
		}
		activity.Captures = append(activity.Captures, item)
	}
	if err := rows.Err(); err != nil {
		return activity, err
	}

	rows, err = s.Pool.Query(ctx, `SELECT id::text,amount,processor_refund_id,created_at FROM payment_refunds WHERE payment_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return activity, err
	}
	defer rows.Close()
	for rows.Next() {
		var item RefundActivity
		if err := rows.Scan(&item.ID, &item.Amount, &item.ProcessorRefundID, &item.CreatedAt); err != nil {
			return activity, err
		}
		activity.Refunds = append(activity.Refunds, item)
	}
	if err := rows.Err(); err != nil {
		return activity, err
	}

	rows, err = s.Pool.Query(ctx, `
		SELECT lj.id::text,lj.event_type,lj.reference_type,lj.reference_id::text,lj.currency,
		       COALESCE(SUM(le.amount) FILTER (WHERE le.direction='DEBIT'),0),
		       COALESCE(SUM(le.amount) FILTER (WHERE le.direction='CREDIT'),0),lj.created_at
		FROM ledger_journals lj
		JOIN ledger_entries le ON le.journal_id=lj.id
		WHERE (lj.reference_type='payment_capture' AND lj.reference_id IN (SELECT id FROM payment_captures WHERE payment_id=$1))
		   OR (lj.reference_type='payment_refund' AND lj.reference_id IN (SELECT id FROM payment_refunds WHERE payment_id=$1))
		GROUP BY lj.id
		ORDER BY lj.created_at`, id)
	if err != nil {
		return activity, err
	}
	defer rows.Close()
	for rows.Next() {
		var item JournalActivity
		if err := rows.Scan(&item.ID, &item.EventType, &item.ReferenceType, &item.ReferenceID, &item.Currency, &item.Debit, &item.Credit, &item.CreatedAt); err != nil {
			return activity, err
		}
		activity.Journals = append(activity.Journals, item)
	}
	if err := rows.Err(); err != nil {
		return activity, err
	}
	return activity, nil
}

func (x *Service) Pay(ctx context.Context, key string, req CreateRequest) (payment.Payment, string, int, error) {
	p, _, err := x.Create(ctx, key, req)
	if err != nil {
		return p, "", 0, err
	}
	p, _, err = x.Authorize(ctx, p.ID, key+"-authorize")
	if err != nil {
		return p, "", 0, err
	}
	return x.Capture(ctx, p.ID, key+"-capture", CaptureRequest{Amount: p.AuthorizedAmount})
}

func (x *Service) Create(ctx context.Context, key string, req CreateRequest) (payment.Payment, int, error) {
	if req.Amount <= 0 || len(req.Currency) != 3 || req.CaptureMethod != "manual" {
		return payment.Payment{}, 0, fmt.Errorf("invalid create request")
	}
	operation := "CREATE_PAYMENT"
	r, err := x.Store.Claim(ctx, key, operation, Hash(req))
	if err != nil {
		return payment.Payment{}, 0, err
	}
	if r.Completed {
		var p payment.Payment
		err = json.Unmarshal(r.Body, &p)
		return p, r.Code, err
	}
	tx, err := x.Store.Pool.Begin(ctx)
	if err != nil {
		return payment.Payment{}, 0, err
	}
	defer tx.Rollback(ctx)
	p, err := scanPayment(tx.QueryRow(ctx, `INSERT INTO payments(amount,currency,status,capture_method,reference) VALUES($1,$2,'CREATED',$3,NULLIF($4,'')) RETURNING `+paymentColumns, req.Amount, req.Currency, req.CaptureMethod, req.Reference))
	if err != nil {
		x.Store.Fail(ctx, key, operation)
		return p, 0, err
	}
	if err = x.Store.Complete(ctx, tx, key, operation, p.ID, 201, p); err != nil {
		return p, 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, 0, err
	}
	return p, 201, nil
}
func (x *Service) Authorize(ctx context.Context, id, key string) (payment.Payment, int, error) {
	op := "AUTHORIZE_PAYMENT"
	r, err := x.Store.Claim(ctx, key, op, Hash(struct{ ID string }{id}))
	if err != nil {
		return payment.Payment{}, 0, err
	}
	if r.Completed {
		var p payment.Payment
		err = json.Unmarshal(r.Body, &p)
		return p, r.Code, err
	}
	tx, err := x.Store.Pool.Begin(ctx)
	if err != nil {
		return payment.Payment{}, 0, err
	}
	defer tx.Rollback(ctx)
	p, err := getForUpdate(ctx, tx, id)
	if err != nil {
		x.Store.Fail(ctx, key, op)
		return p, 0, err
	}
	if err = payment.CanAuthorize(p.Status); err != nil {
		x.Store.Fail(ctx, key, op)
		return p, 0, err
	}
	processor := x.Processor
	if selector, ok := x.Processor.(interface{ ProcessorForAmount(int64) payment.Processor }); ok {
		processor = selector.ProcessorForAmount(p.Amount)
	}
	ref, err := processor.Authorize(ctx, p.ID, p.Amount, p.Currency)
	if err != nil {
		x.Store.Fail(ctx, key, op)
		return p, 0, err
	}
	p.Status = payment.Authorized
	p.AuthorizedAmount = p.Amount
	p.Processor = "unknown"
	if named, ok := processor.(interface{ Name() string }); ok {
		p.Processor = named.Name()
	}
	p.ProcessorPaymentID = ref
	_, err = tx.Exec(ctx, `UPDATE payments SET status=$2,authorized_amount=$3,processor=$4,processor_payment_id=$5,updated_at=now() WHERE id=$1`, p.ID, p.Status, p.AuthorizedAmount, p.Processor, p.ProcessorPaymentID)
	if err != nil {
		return p, 0, err
	}
	if err = x.Store.Complete(ctx, tx, key, op, p.ID, 200, p); err != nil {
		return p, 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, 0, err
	}
	return p, 200, nil
}
func (x *Service) Capture(ctx context.Context, id, key string, req CaptureRequest) (payment.Payment, string, int, error) {
	op := "CAPTURE_PAYMENT"
	hash := Hash(struct {
		ID     string
		Amount int64
	}{id, req.Amount})
	r, err := x.Store.Claim(ctx, key, op, hash)
	if err != nil {
		return payment.Payment{}, "", 0, err
	}
	if r.Completed {
		var v struct {
			Payment            payment.Payment `json:"payment"`
			ProcessorCaptureID string          `json:"processor_capture_id"`
		}
		err = json.Unmarshal(r.Body, &v)
		return v.Payment, v.ProcessorCaptureID, r.Code, err
	}
	tx, err := x.Store.Pool.Begin(ctx)
	if err != nil {
		return payment.Payment{}, "", 0, err
	}
	defer tx.Rollback(ctx)
	p, err := getForUpdate(ctx, tx, id)
	if err != nil {
		x.Store.Fail(ctx, key, op)
		return p, "", 0, err
	}
	next, err := payment.CaptureStatus(p.Status, p.AuthorizedAmount, p.CapturedAmount, req.Amount)
	if err != nil {
		x.Store.Fail(ctx, key, op)
		return p, "", 0, err
	}
	processor := x.Processor
	if resolver, ok := x.Processor.(interface {
		ProcessorByName(string) (payment.Processor, bool)
	}); ok {
		var found bool
		processor, found = resolver.ProcessorByName(p.Processor)
		if !found {
			x.Store.Fail(ctx, key, op)
			return p, "", 0, fmt.Errorf("processor %q is not configured", p.Processor)
		}
	}
	if capable, ok := processor.(interface{ SupportsPartialCapture() bool }); ok && !capable.SupportsPartialCapture() && req.Amount != p.AuthorizedAmount-p.CapturedAmount {
		x.Store.Fail(ctx, key, op)
		return p, "", 0, fmt.Errorf("%w: capture the full remaining authorized amount", ErrPartialCaptureUnsupported)
	}
	finalCapture := p.CapturedAmount+req.Amount == p.AuthorizedAmount
	var processorID string
	if captureProcessor, ok := processor.(interface {
		CaptureFinal(context.Context, string, int64, string, bool) (string, error)
	}); ok {
		processorID, err = captureProcessor.CaptureFinal(ctx, p.ProcessorPaymentID, req.Amount, key, finalCapture)
	} else {
		processorID, err = processor.Capture(ctx, p.ProcessorPaymentID, req.Amount, key)
	}
	if err != nil {
		x.Store.Fail(ctx, key, op)
		return p, "", 0, err
	}
	var captureID string
	err = tx.QueryRow(ctx, `INSERT INTO payment_captures(payment_id,amount,processor_capture_id) VALUES($1,$2,$3) RETURNING id::text`, p.ID, req.Amount, processorID).Scan(&captureID)
	if err != nil {
		return p, "", 0, err
	}
	p.CapturedAmount += req.Amount
	p.Status = next
	_, err = tx.Exec(ctx, `UPDATE payments SET captured_amount=$2,status=$3,updated_at=now() WHERE id=$1`, p.ID, p.CapturedAmount, p.Status)
	if err != nil {
		return p, "", 0, err
	}
	if err = x.insertJournal(ctx, tx, captureID, p.Currency, req.Amount); err != nil {
		return p, "", 0, err
	}
	body := struct {
		Payment            payment.Payment `json:"payment"`
		ProcessorCaptureID string          `json:"processor_capture_id"`
	}{p, processorID}
	if err = x.Store.Complete(ctx, tx, key, op, p.ID, 200, body); err != nil {
		return p, "", 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, "", 0, err
	}
	return p, processorID, 200, nil
}

func (x *Service) Refund(ctx context.Context, id, key string, req RefundRequest) (payment.Payment, string, int, error) {
	op := "REFUND_PAYMENT"
	hash := Hash(struct {
		ID     string
		Amount int64
	}{id, req.Amount})
	r, err := x.Store.Claim(ctx, key, op, hash)
	if err != nil {
		return payment.Payment{}, "", 0, err
	}
	if r.Completed {
		var v struct {
			Payment           payment.Payment `json:"payment"`
			ProcessorRefundID string          `json:"processor_refund_id"`
		}
		err = json.Unmarshal(r.Body, &v)
		return v.Payment, v.ProcessorRefundID, r.Code, err
	}
	tx, err := x.Store.Pool.Begin(ctx)
	if err != nil {
		return payment.Payment{}, "", 0, err
	}
	defer tx.Rollback(ctx)
	p, err := getForUpdate(ctx, tx, id)
	if err != nil {
		x.Store.Fail(ctx, key, op)
		return p, "", 0, err
	}
	if req.Amount <= 0 {
		x.Store.Fail(ctx, key, op)
		return p, "", 0, payment.ErrInvalidAmount
	}
	if req.Amount > p.CapturedAmount-p.RefundedAmount {
		x.Store.Fail(ctx, key, op)
		return p, "", 0, payment.ErrOverRefund
	}
	processor := x.Processor
	if resolver, ok := x.Processor.(interface {
		ProcessorByName(string) (payment.Processor, bool)
	}); ok {
		processor, ok = resolver.ProcessorByName(p.Processor)
		if !ok {
			x.Store.Fail(ctx, key, op)
			return p, "", 0, fmt.Errorf("processor %q is not configured", p.Processor)
		}
	}
	processorID, err := processor.Refund(ctx, p.ProcessorPaymentID, req.Amount, key, p.Currency)
	if err != nil {
		x.Store.Fail(ctx, key, op)
		return p, "", 0, err
	}
	var refundID string
	err = tx.QueryRow(ctx, `INSERT INTO payment_refunds(payment_id,amount,processor_refund_id) VALUES($1,$2,$3) RETURNING id::text`, p.ID, req.Amount, processorID).Scan(&refundID)
	if err != nil {
		return p, "", 0, err
	}
	p.RefundedAmount += req.Amount
	_, err = tx.Exec(ctx, `UPDATE payments SET refunded_amount=$2,updated_at=now() WHERE id=$1`, p.ID, p.RefundedAmount)
	if err != nil {
		return p, "", 0, err
	}
	if err = x.insertRefundJournal(ctx, tx, refundID, p.Currency, req.Amount); err != nil {
		return p, "", 0, err
	}
	body := struct {
		Payment           payment.Payment `json:"payment"`
		ProcessorRefundID string          `json:"processor_refund_id"`
	}{p, processorID}
	if err = x.Store.Complete(ctx, tx, key, op, p.ID, 200, body); err != nil {
		return p, "", 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return p, "", 0, err
	}
	return p, processorID, 200, nil
}

func (x *Service) insertRefundJournal(ctx context.Context, tx pgx.Tx, refundID, currency string, amount int64) error {
	var journal, payable, receivable string
	if err := tx.QueryRow(ctx, `INSERT INTO ledger_journals(event_type,reference_type,reference_id,currency) VALUES('PAYMENT_REFUNDED','payment_refund',$1,$2) RETURNING id::text`, refundID, currency).Scan(&journal); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `INSERT INTO ledger_accounts(code,name,account_type,currency) VALUES($1,$1,'LIABILITY',$2) ON CONFLICT(code) DO UPDATE SET code=EXCLUDED.code RETURNING id::text`, `MERCHANT_PAYABLE_`+currency, currency).Scan(&payable); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `INSERT INTO ledger_accounts(code,name,account_type,currency) VALUES($1,$1,'ASSET',$2) ON CONFLICT(code) DO UPDATE SET code=EXCLUDED.code RETURNING id::text`, `PROCESSOR_RECEIVABLE_`+currency, currency).Scan(&receivable); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO ledger_entries(journal_id,account_id,direction,amount) VALUES($1,$2,'DEBIT',$3),($1,$4,'CREDIT',$3)`, journal, payable, amount, receivable)
	return err
}

func (x *Service) insertJournal(ctx context.Context, tx pgx.Tx, captureID, currency string, amount int64) error {
	var j, recv, payable string
	err := tx.QueryRow(ctx, `INSERT INTO ledger_journals(event_type,reference_type,reference_id,currency) VALUES('PAYMENT_CAPTURED','payment_capture',$1,$2) RETURNING id::text`, captureID, currency).Scan(&j)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `INSERT INTO ledger_accounts(code,name,account_type,currency) VALUES($1,$1,'ASSET',$2) ON CONFLICT(code) DO UPDATE SET code=EXCLUDED.code RETURNING id::text`, `PROCESSOR_RECEIVABLE_`+currency, currency).Scan(&recv)
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `INSERT INTO ledger_accounts(code,name,account_type,currency) VALUES($1,$1,'LIABILITY',$2) ON CONFLICT(code) DO UPDATE SET code=EXCLUDED.code RETURNING id::text`, `MERCHANT_PAYABLE_`+currency, currency).Scan(&payable)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO ledger_entries(journal_id,account_id,direction,amount) VALUES($1,$2,'DEBIT',$3),($1,$4,'CREDIT',$3)`, j, recv, amount, payable)
	return err
}
