CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE payments (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), amount BIGINT NOT NULL CHECK(amount > 0), currency CHAR(3) NOT NULL,
 authorized_amount BIGINT NOT NULL DEFAULT 0 CHECK(authorized_amount >= 0), captured_amount BIGINT NOT NULL DEFAULT 0 CHECK(captured_amount >= 0),
 status VARCHAR NOT NULL, capture_method VARCHAR NOT NULL, reference VARCHAR NULL, processor VARCHAR NULL, processor_payment_id VARCHAR NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(captured_amount <= authorized_amount), CHECK(authorized_amount <= amount)
);
CREATE TABLE payment_captures (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), payment_id UUID NOT NULL REFERENCES payments(id), amount BIGINT NOT NULL CHECK(amount > 0), processor_capture_id VARCHAR NOT NULL UNIQUE, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE idempotency_records (idempotency_key VARCHAR NOT NULL, operation VARCHAR NOT NULL, request_hash VARCHAR NOT NULL, resource_id UUID NULL, response_code INT NULL, response_body JSONB NULL, status VARCHAR NOT NULL CHECK(status IN ('IN_PROGRESS','COMPLETED','FAILED')), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(idempotency_key, operation));
CREATE TABLE ledger_accounts (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), code VARCHAR NOT NULL UNIQUE, name VARCHAR NOT NULL, account_type VARCHAR NOT NULL, currency CHAR(3) NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE ledger_journals (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), event_type VARCHAR NOT NULL, reference_type VARCHAR NOT NULL, reference_id UUID NOT NULL, currency CHAR(3) NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(event_type, reference_id));
CREATE TABLE ledger_entries (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), journal_id UUID NOT NULL REFERENCES ledger_journals(id), account_id UUID NOT NULL REFERENCES ledger_accounts(id), direction VARCHAR NOT NULL CHECK(direction IN ('DEBIT','CREDIT')), amount BIGINT NOT NULL CHECK(amount > 0), created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE OR REPLACE FUNCTION forbid_ledger_mutation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'ledger is append-only'; END; $$;
CREATE TRIGGER ledger_journals_immutable BEFORE UPDATE OR DELETE ON ledger_journals FOR EACH ROW EXECUTE FUNCTION forbid_ledger_mutation();
CREATE TRIGGER ledger_entries_immutable BEFORE UPDATE OR DELETE ON ledger_entries FOR EACH ROW EXECUTE FUNCTION forbid_ledger_mutation();
CREATE OR REPLACE FUNCTION ensure_journal_balanced() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM ledger_entries WHERE journal_id = NEW.journal_id GROUP BY journal_id HAVING SUM(CASE direction WHEN 'DEBIT' THEN amount ELSE -amount END) <> 0) THEN
    RAISE EXCEPTION 'journal % is not balanced', NEW.journal_id;
  END IF;
  RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER ledger_journal_balanced
AFTER INSERT ON ledger_entries DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION ensure_journal_balanced();
