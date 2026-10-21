CREATE TABLE wallet_ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    transaction_id UUID NOT NULL REFERENCES wager_transactions(id),
    direction TEXT NOT NULL CHECK (direction IN ('CREDIT', 'DEBIT')),
    currency TEXT NOT NULL CHECK (currency = 'BRL'),
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    balance_before_cents BIGINT NOT NULL CHECK (balance_before_cents >= 0),
    balance_after_cents BIGINT NOT NULL CHECK (balance_after_cents >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (wallet_id, transaction_id),

    CHECK (
        (direction = 'CREDIT'
        AND balance_after_cents = balance_before_cents + amount_cents)
        OR
        (direction = 'DEBIT'
        AND balance_after_cents = balance_before_cents - amount_cents)
    )
);