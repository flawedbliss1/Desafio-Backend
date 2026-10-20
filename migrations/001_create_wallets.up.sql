CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id UUID NOT NULL,
    currency TEXT NOT NULL CHECK (currency = 'BRL'),
    balance_cents BIGINT NOT NULL DEFAULT 0 CHECK (balance_cents >= 0),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (player_id, currency)

);