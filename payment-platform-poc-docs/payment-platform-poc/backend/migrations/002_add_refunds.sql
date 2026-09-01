ALTER TABLE payments ADD COLUMN IF NOT EXISTS refunded_amount BIGINT NOT NULL DEFAULT 0 CHECK(refunded_amount >= 0);
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_refunded_amount_check;
ALTER TABLE payments ADD CONSTRAINT payments_refunded_amount_check CHECK(refunded_amount <= captured_amount);
CREATE TABLE IF NOT EXISTS payment_refunds (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), payment_id UUID NOT NULL REFERENCES payments(id), amount BIGINT NOT NULL CHECK(amount > 0), processor_refund_id VARCHAR NOT NULL UNIQUE, created_at TIMESTAMPTZ NOT NULL DEFAULT now());