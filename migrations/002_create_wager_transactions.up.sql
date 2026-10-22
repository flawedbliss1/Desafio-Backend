CREATE TABLE wager_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    player_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('OPENING', 'BET')),
    currency TEXT NOT NULL CHECK (currency = 'BRL'),
    amount_cents BIGINT NOT NULL CHECK (amount_cents >= 0),
    state TEXT NOT NULL CHECK (state IN ('PENDING', 'PROCESSED', 'REJECTED', 'FAILED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);