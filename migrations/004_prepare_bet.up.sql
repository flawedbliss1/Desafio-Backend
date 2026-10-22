ALTER TABLE wager_transactions
    ADD COLUMN provider_id TEXT, 
    ADD COLUMN external_transaction_id TEXT, 
    ADD COLUMN idempotency_key TEXT,
    ADD COLUMN payload_hash TEXT,
    ADD COLUMN round_id TEXT,
    ADD COLUMN game_id TEXT,
    ADD COLUMN balance_after_cents BIGINT CHECK (balance_after_cents >= 0),
    ADD COLUMN failure_code TEXT,
    ADD CONSTRAINT wager_transactions_provider_external_key
       UNIQUE (provider_id, external_transaction_id),
    ADD CONSTRAINT wager_transactions_provider_idempotency_key
       UNIQUE (provider_id, idempotency_key),
    ADD CONSTRAINT wager_transactions_origin_check
       CHECK (amount_cents > 0 AND (
       (kind = 'OPENING' AND 
       provider_id IS NULL AND 
       external_transaction_id IS NULL AND 
       idempotency_key IS NULL AND 
       payload_hash IS NULL AND 
       round_id IS NULL AND 
       game_id IS NULL) OR 
       (kind = 'BET' AND
        provider_id IS NOT NULL AND 
        external_transaction_id IS NOT NULL AND 
        idempotency_key IS NOT NULL AND
         payload_hash IS NOT NULL AND 
         round_id IS NOT NULL AND 
         game_id IS NOT NULL)));