package main

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Wallet struct {
	ID           string
	PlayerID     string
	Currency     string
	BalanceCents int64
	Version      int64
}

type MoneyRequest struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type CreateWalletRequest struct {
	PlayerID       string       `json:"playerId"`
	InitialBalance MoneyRequest `json:"initialBalance"`
}

func findWalletBalance(ctx context.Context, pool *pgxpool.Pool, walletID string) (int64, error) {
	var balanceCents int64
	query := "SELECT balance_cents FROM wallets WHERE id = $1"

	err := pool.QueryRow(ctx, query, walletID).Scan(&balanceCents)
	if err != nil {
		return 0, err
	}
	return balanceCents, nil
}

func findWallet(ctx context.Context, pool *pgxpool.Pool, walletID string) (Wallet, error) {
	var wallet Wallet
	query := "SELECT id::text, player_id::text, currency, balance_cents, version FROM wallets WHERE id = $1"
	err := pool.QueryRow(ctx, query, walletID).Scan(
		&wallet.ID,
		&wallet.PlayerID,
		&wallet.Currency,
		&wallet.BalanceCents,
		&wallet.Version,
	)
	if err != nil {
		return Wallet{}, err
	}
	return wallet, nil
}

func createWallet(ctx context.Context, pool *pgxpool.Pool, playerID string, currency string, initialCents int64) (Wallet, error) {
	var wallets Wallet
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Wallet{}, err
	}
	defer tx.Rollback(ctx)
	query := "INSERT INTO wallets (player_id, currency, balance_cents) VALUES ($1, $2, $3) RETURNING id::text, player_id::text, currency, balance_cents, version"
	err = tx.QueryRow(ctx, query, playerID, currency, initialCents).Scan(
		&wallets.ID,
		&wallets.PlayerID,
		&wallets.Currency,
		&wallets.BalanceCents,
		&wallets.Version,
	)
	if err != nil {
		return Wallet{}, err
	}
	if initialCents > 0 {
		var transactionID string
		query = "INSERT INTO wager_transactions (wallet_id, player_id, kind, currency, amount_cents, state) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text "
		err = tx.QueryRow(
			ctx,
			query,
			wallets.ID,
			wallets.PlayerID,
			"OPENING",
			wallets.Currency,
			initialCents,
			"PROCESSED",
		).Scan(&transactionID)
		if err != nil {
			return Wallet{}, err
		}
		query = "INSERT INTO wallet_ledger_entries (wallet_id, transaction_id, direction, currency, amount_cents, balance_before_cents, balance_after_cents) VALUES ($1, $2, $3, $4, $5, $6, $7)"
		_, err = tx.Exec(
			ctx,
			query,
			wallets.ID,
			transactionID,
			"CREDIT",
			wallets.Currency,
			initialCents,
			0,
			wallets.BalanceCents,
		)
		if err != nil {
			return Wallet{}, err
		}
	}
	err = tx.Commit(ctx)
	if err != nil {
		return Wallet{}, err
	}
	return wallets, nil
}

func findWalletForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	walletID string,
) (Wallet, error) {
	var wallet Wallet
	query := "SELECT id::text, player_id::text, currency, balance_cents, version FROM wallets WHERE id = $1 FOR UPDATE"
	err := tx.QueryRow(ctx, query, walletID).Scan(
		&wallet.ID,
		&wallet.PlayerID,
		&wallet.Currency,
		&wallet.BalanceCents,
		&wallet.Version,
	)
	if err != nil {
		return Wallet{}, err
	}
	return wallet, nil

}

func updateWalletBalance(ctx context.Context, tx pgx.Tx, walletID string, BalanceAfterCents int64) error {

	query := "UPDATE wallets SET balance_cents = $2, version = version + 1, updated_at = now()  WHERE id = $1"
	_, err := tx.Exec(ctx, query, walletID, BalanceAfterCents)
	return err

}

func insertBetLedgerEntry(ctx context.Context, tx pgx.Tx, wallet Wallet, transactionID string, amountCents, BalanceAfterCents int64) error {

	query := "INSERT INTO wallet_ledger_entries (wallet_id, transaction_id, direction, currency, amount_cents, balance_before_cents, balance_after_cents) VALUES ($1, $2, 'DEBIT', $3, $4, $5, $6)"
	_, err := tx.Exec(ctx, query, wallet.ID, transactionID, wallet.Currency, amountCents, wallet.BalanceCents, BalanceAfterCents)
	return err

}
