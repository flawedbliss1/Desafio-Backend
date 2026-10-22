package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CreateWagerTransactionRequest struct {
	ProviderID            string       `json:"providerId"`
	ExternalTransactionID string       `json:"externalTransactionId"`
	PlayerID              string       `json:"playerId"`
	WalletID              string       `json:"walletId"`
	RoundID               string       `json:"roundId"`
	GameID                string       `json:"gameId"`
	Kind                  string       `json:"kind"`
	Money                 MoneyRequest `json:"money"`
}

type StoredWager struct {
	ID                    string
	ExternalTransactionID string
	IdempotencyKey        string
	PayloadHash           string
	State                 string
	Currency              string
	BalanceAfterCents     *int64
	FailureCode           *string
}

var (
	ErrIdempotencyConflict         = errors.New("idempotency_conflict")
	ErrWagerResultUnavailable      = errors.New("wager_result_unavailable")
	ErrExternalTransactionConflict = errors.New("external_transaction_conflict")
	ErrWalletNotFound              = errors.New("wallet_not_found")
	ErrWalletPlayerMismatch        = errors.New("wallet_player_mismatch")
	ErrCurrencyMismatch            = errors.New("currency_mismatch")
)

func canonicalWagerPayload(
	req CreateWagerTransactionRequest,
	amountCents int64,
) ([]byte, error) {

	amountText := fmt.Sprintf("%d.%02d", amountCents/100, amountCents%100)
	mapa := map[string]any{
		"providerId":            req.ProviderID,
		"externalTransactionId": req.ExternalTransactionID,
		"playerId":              req.PlayerID,
		"walletId":              req.WalletID,
		"roundId":               req.RoundID,
		"gameId":                req.GameID,
		"kind":                  req.Kind,
		"money": map[string]any{
			"amount":   amountText,
			"currency": req.Money.Currency,
		},
	}
	bytes, err := json.Marshal(mapa)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func hashWagerPayload(
	req CreateWagerTransactionRequest,
	amountCents int64,
) (string, error) {
	payload, err := canonicalWagerPayload(req, amountCents)
	if err != nil {
		return "", err
	}
	hashBytes := sha256.Sum256(payload)
	hashText := fmt.Sprintf("%x", hashBytes)
	return hashText, nil
}

func insertPendingBet(
	ctx context.Context,
	tx pgx.Tx,
	req CreateWagerTransactionRequest,
	amountCents int64,
	idempotencyKey string,
	payloadHash string,
) (string, error) {

	query := `INSERT INTO wager_transactions (
		wallet_id, player_id, kind, currency, amount_cents, state, provider_id, external_transaction_id, idempotency_key,
		payload_hash, round_id, game_id
    )
		VALUES (
		$1, $2, 'BET', $3, $4, 'PENDING',
		$5, $6, $7, $8, $9, $10
		)
		ON CONFLICT DO NOTHING
		RETURNING id::text
		`
	var transactionID string
	err := tx.QueryRow(ctx, query, req.WalletID, req.PlayerID, req.Money.Currency, amountCents,
		req.ProviderID, req.ExternalTransactionID,
		idempotencyKey, payloadHash,
		req.RoundID, req.GameID).Scan(&transactionID)
	if err != nil {
		return "", err
	}
	return transactionID, nil
}

func findWagerByKey(
	ctx context.Context,
	tx pgx.Tx,
	providerID string,
	idempotencyKey string,
) (StoredWager, error) {

	query := `
	SELECT
		id::text,
		external_transaction_id,
		idempotency_key,
		payload_hash,
		state,
		currency,
		balance_after_cents,
		failure_code
	FROM wager_transactions
	WHERE provider_id = $1
	  AND idempotency_key = $2
`
	var wager StoredWager

	err := tx.QueryRow(ctx, query, providerID, idempotencyKey).Scan(
		&wager.ID,
		&wager.ExternalTransactionID,
		&wager.IdempotencyKey,
		&wager.PayloadHash,
		&wager.State,
		&wager.Currency,
		&wager.BalanceAfterCents,
		&wager.FailureCode,
	)
	if err != nil {
		return StoredWager{}, err
	}
	return wager, nil
}

func findWagerByExternalID(
	ctx context.Context,
	tx pgx.Tx,
	providerID string,
	ExternalTransactionID string,
) (StoredWager, error) {

	query := `SELECT
		id::text,
		external_transaction_id,
		idempotency_key,
		payload_hash,
		state,
		currency,
		balance_after_cents,
		failure_code
	FROM wager_transactions
	WHERE provider_id = $1
	  AND external_transaction_id = $2`

	var wager StoredWager
	err := tx.QueryRow(ctx, query, providerID, ExternalTransactionID).Scan(
		&wager.ID,
		&wager.ExternalTransactionID,
		&wager.IdempotencyKey,
		&wager.PayloadHash,
		&wager.State,
		&wager.Currency,
		&wager.BalanceAfterCents,
		&wager.FailureCode,
	)
	if err != nil {
		return StoredWager{}, err
	}
	return wager, nil
}

func processBet(
	ctx context.Context,
	pool *pgxpool.Pool,
	req CreateWagerTransactionRequest,
	amountCents int64,
	idempotencyKey string,
	payloadHash string,
) (StoredWager, bool, error) {

	tx, err := pool.Begin(ctx)
	if err != nil {
		return StoredWager{}, false, err
	}
	defer tx.Rollback(ctx)

	wager, err := findWagerByKey(ctx, tx, req.ProviderID, idempotencyKey)
	if err == nil {
		if wager.PayloadHash != payloadHash {
			return StoredWager{}, false, ErrIdempotencyConflict
		}

		if (wager.State != "PROCESSED" && wager.State != "REJECTED") ||
			wager.BalanceAfterCents == nil {
			return StoredWager{}, false, ErrWagerResultUnavailable
		}

		return wager, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return StoredWager{}, false, err
	}

	wager, err = findWagerByExternalID(
		ctx,
		tx,
		req.ProviderID,
		req.ExternalTransactionID,
	)
	if err == nil {
		if wager.IdempotencyKey != idempotencyKey {
			return StoredWager{}, false, ErrExternalTransactionConflict
		}

		if wager.PayloadHash != payloadHash {
			return StoredWager{}, false, ErrIdempotencyConflict
		}

		if (wager.State != "PROCESSED" && wager.State != "REJECTED") ||
			wager.BalanceAfterCents == nil {
			return StoredWager{}, false, ErrWagerResultUnavailable
		}

		return wager, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return StoredWager{}, false, err
	}

	wallet, err := findWalletForUpdate(ctx, tx, req.WalletID)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredWager{}, false, ErrWalletNotFound
	}
	if err != nil {
		return StoredWager{}, false, err
	}

	if wallet.PlayerID != req.PlayerID {
		return StoredWager{}, false, ErrWalletPlayerMismatch
	}

	if wallet.Currency != req.Money.Currency {
		return StoredWager{}, false, ErrCurrencyMismatch
	}

	wager.ID, err = insertPendingBet(
		ctx,
		tx,
		req,
		amountCents,
		idempotencyKey,
		payloadHash,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		wager, err = findWagerByKey(
			ctx,
			tx,
			req.ProviderID,
			idempotencyKey,
		)
		if err == nil {
			if wager.PayloadHash != payloadHash {
				return StoredWager{}, false, ErrIdempotencyConflict
			}

			if (wager.State != "PROCESSED" && wager.State != "REJECTED") ||
				wager.BalanceAfterCents == nil {
				return StoredWager{}, false, ErrWagerResultUnavailable
			}

			return wager, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return StoredWager{}, false, err
		}

		wager, err = findWagerByExternalID(
			ctx,
			tx,
			req.ProviderID,
			req.ExternalTransactionID,
		)
		if err == nil {
			if wager.IdempotencyKey != idempotencyKey {
				return StoredWager{}, false, ErrExternalTransactionConflict
			}

			if wager.PayloadHash != payloadHash {
				return StoredWager{}, false, ErrIdempotencyConflict
			}

			if (wager.State != "PROCESSED" && wager.State != "REJECTED") ||
				wager.BalanceAfterCents == nil {
				return StoredWager{}, false, ErrWagerResultUnavailable
			}

			return wager, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return StoredWager{}, false, err
		}

		return StoredWager{}, false, ErrWagerResultUnavailable
	}

	if err != nil {
		return StoredWager{}, false, err
	}

	if amountCents > wallet.BalanceCents {
		failureCode := "insufficient_funds"
		wager, err = updateBetResult(ctx, tx, wager.ID, "REJECTED", wallet.BalanceCents, &failureCode)
		if err != nil {
			return StoredWager{}, false, err
		}

		if err := tx.Commit(ctx); err != nil {
			return StoredWager{}, false, err
		}
		return wager, false, nil
	}

	balanceAfterCents := wallet.BalanceCents - amountCents
	err = updateWalletBalance(ctx, tx, wallet.ID, balanceAfterCents)
	if err != nil {
		return StoredWager{}, false, err
	}
	err = insertBetLedgerEntry(ctx, tx, wallet, wager.ID, amountCents, balanceAfterCents)
	if err != nil {
		return StoredWager{}, false, err
	}
	wager, err = updateBetResult(ctx, tx, wager.ID, "PROCESSED", balanceAfterCents, nil)
	if err != nil {
		return StoredWager{}, false, err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return StoredWager{}, false, err
	}
	return wager, false, nil
}

func updateBetResult(
	ctx context.Context,
	tx pgx.Tx,
	transactionID string,
	state string,
	balanceAfterCents int64,
	failureCode *string,
) (StoredWager, error) {
	query := `UPDATE wager_transactions
SET state = $2,
	balance_after_cents = $3,
	failure_code = $4,
	updated_at = now()
WHERE id = $1
	AND kind = 'BET'
	AND state = 'PENDING'
RETURNING
	id::text,
	external_transaction_id,
	idempotency_key,
	payload_hash,
	state,
	currency,
	balance_after_cents,
	failure_code`

	var wager StoredWager
	err := tx.QueryRow(ctx, query, transactionID, state, balanceAfterCents, failureCode).Scan(
		&wager.ID,
		&wager.ExternalTransactionID,
		&wager.IdempotencyKey,
		&wager.PayloadHash,
		&wager.State,
		&wager.Currency,
		&wager.BalanceAfterCents,
		&wager.FailureCode,
	)
	if err != nil {
		return StoredWager{}, err
	}
	return wager, nil
}
