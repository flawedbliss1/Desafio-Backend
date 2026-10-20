package main

import (
	"context"

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

func createWallet(ctx context.Context, pool *pgxpool.Pool, playerID string, currency string) (Wallet, error) {
	var wallets Wallet
	query := "INSERT INTO wallets (player_id, currency) VALUES ($1, $2) RETURNING id::text, player_id::text, currency, balance_cents, version"
	err := pool.QueryRow(ctx, query, playerID, currency).Scan(
		&wallets.ID,
		&wallets.PlayerID,
		&wallets.Currency,
		&wallets.BalanceCents,
		&wallets.Version,
	)
	if err != nil {
		return Wallet{}, err
	}
	return wallets, nil
}
