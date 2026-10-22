ALTER TABLE wager_transactions

ADD CONSTRAINT wager_transactions_result_check
        CHECK (
            kind = 'OPENING'
            OR (
                kind = 'BET'
                AND (
                    state = 'PENDING' AND balance_after_cents IS NULL AND failure_code IS NULL 
                    OR (
                        state = 'PROCESSED' AND balance_after_cents IS NOT NULL AND failure_code IS NULL
                        OR (
                            state = 'REJECTED' AND balance_after_cents IS NOT NULL AND failure_code IS NOT NULL 
                            OR (
                                STATE = 'FAILED' AND failure_code IS NOT NULL
                            )
                        )
                    )
                )
            )
        );