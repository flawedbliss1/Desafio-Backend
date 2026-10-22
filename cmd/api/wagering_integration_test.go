package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProcessBetApproved(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Faltando TEST_DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var playerID string
	err = pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&playerID)
	if err != nil {
		t.Fatal(err)
	}

	wallet, err := createWallet(ctx, pool, playerID, "BRL", 10000)
	if err != nil {
		t.Fatalf("criar carteira: %v", err)
	}

	req := CreateWagerTransactionRequest{
		ProviderID:            "provider-teste",
		ExternalTransactionID: "bet-" + wallet.ID,
		PlayerID:              playerID,
		WalletID:              wallet.ID,
		RoundID:               "round-teste",
		GameID:                "game-teste",
		Kind:                  "BET",
		Money: MoneyRequest{
			Amount:   "10.00",
			Currency: "BRL",
		},
	}

	amountCents, err := parseAmountCents(req.Money.Amount)
	if err != nil {
		t.Fatal(err)
	}

	payloadHash, err := hashWagerPayload(req, amountCents)
	if err != nil {
		t.Fatal(err)
	}

	wager, replay, err := processBet(
		ctx,
		pool,
		req,
		amountCents,
		"key-"+wallet.ID,
		payloadHash,
	)
	if err != nil {
		t.Fatalf("processar BET: %v", err)
	}

	if replay {
		t.Fatal("Sem replay")
	}
	if wager.State != "PROCESSED" {
		t.Fatalf("recebi %s", wager.State)
	}
	if wager.BalanceAfterCents == nil {
		t.Fatal("sem saldo resultante")
	}
	if *wager.BalanceAfterCents != 9000 {
		t.Fatalf("recebi %d", *wager.BalanceAfterCents)
	}

	repeatedWager, repeatedReplay, err := processBet(
		ctx,
		pool,
		req,
		amountCents,
		"key-"+wallet.ID,
		payloadHash,
	)

	if err != nil {
		t.Fatalf("erro %v", err)
	}
	if !repeatedReplay {
		t.Fatalf("false")
	}
	if repeatedWager.ID != wager.ID {
		t.Errorf(wager.ID, repeatedWager.ID)
	}

	if repeatedWager.State != "PROCESSED" {
		t.Errorf("%v", repeatedWager.State)
	}
	if repeatedWager.BalanceAfterCents == nil || *repeatedWager.BalanceAfterCents != 9000 {
		t.Errorf("%v", repeatedWager.BalanceAfterCents)
	}

	updatedWallet, err := findWallet(ctx, pool, wallet.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updatedWallet.BalanceCents != 9000 || updatedWallet.Version != 2 {
		t.Fatalf(
			"recebi saldo %d e versão %d",
			updatedWallet.BalanceCents,
			updatedWallet.Version,
		)
	}

	var storedState string
	var storedBalance int64
	err = pool.QueryRow(ctx, `
		SELECT state, balance_after_cents
		FROM wager_transactions
		WHERE id = $1
	`, wager.ID).Scan(&storedState, &storedBalance)
	if err != nil {
		t.Fatal(err)
	}

	if storedState != "PROCESSED" || storedBalance != 9000 {
		t.Fatalf("resultado inesperado no banco: estado=%s saldo=%d", storedState, storedBalance)
	}

	var debitCount int64
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM wallet_ledger_entries
		WHERE wallet_id = $1
		  AND transaction_id = $2
		  AND direction = 'DEBIT'
	`, wallet.ID, wager.ID).Scan(&debitCount)
	if err != nil {
		t.Fatal(err)
	}

	if debitCount != 1 {
		t.Fatalf("esperava um lançamento DEBIT, recebi %d", debitCount)
	}

	t.Logf("walletId=%s transactionId=%s", wallet.ID, wager.ID)
}

func TestProcessBetInsufficientFunds(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Faltando TEST_DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var playerID string
	err = pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&playerID)
	if err != nil {
		t.Fatal(err)
	}

	wallet, err := createWallet(ctx, pool, playerID, "BRL", 10000)
	if err != nil {
		t.Fatalf("criar carteira: %v", err)
	}

	req := CreateWagerTransactionRequest{
		ProviderID:            "provider-teste",
		ExternalTransactionID: "bet-" + wallet.ID,
		PlayerID:              playerID,
		WalletID:              wallet.ID,
		RoundID:               "round-teste",
		GameID:                "game-teste",
		Kind:                  "BET",
		Money: MoneyRequest{
			Amount:   "150.00",
			Currency: "BRL",
		},
	}

	amountCents, err := parseAmountCents(req.Money.Amount)
	if err != nil {
		t.Fatal(err)
	}

	payloadHash, err := hashWagerPayload(req, amountCents)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf(
		"amount=%q | amountCents=%d | saldoInicial=%d",
		req.Money.Amount,
		amountCents,
		wallet.BalanceCents,
	)
	wager, replay, err := processBet(
		ctx,
		pool,
		req,
		amountCents,
		"key-"+wallet.ID,
		payloadHash,
	)
	if err != nil {
		t.Fatalf("processar BET: %v", err)
	}

	if replay {
		t.Fatal("Sem replay")
	}
	if wager.State != "REJECTED" {
		t.Fatalf("recebi %s", wager.State)
	}
	if wager.BalanceAfterCents == nil {
		t.Fatal("sem saldo resultante")
	}
	if *wager.BalanceAfterCents != 10000 {
		t.Fatalf("recebi %d", *wager.BalanceAfterCents)
	}
	if wager.FailureCode == nil ||
		*wager.FailureCode != "insufficient_funds" {
		t.Fatal("esperava o código insufficient_funds")
	}

	repeatedWager, repeatedReplay, err := processBet(
		ctx,
		pool,
		req,
		amountCents,
		"key-"+wallet.ID,
		payloadHash,
	)

	if err != nil {
		t.Fatalf("erro %v", err)
	}
	if !repeatedReplay {
		t.Fatalf("false")
	}
	if repeatedWager.ID != wager.ID {
		t.Errorf(wager.ID, repeatedWager.ID)
	}

	if repeatedWager.State != "REJECTED" {
		t.Errorf("%v", repeatedWager.State)
	}
	if repeatedWager.BalanceAfterCents == nil || *repeatedWager.BalanceAfterCents != 10000 {
		t.Errorf("%v", repeatedWager.BalanceAfterCents)
	}

	updatedWallet, err := findWallet(ctx, pool, wallet.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updatedWallet.BalanceCents != 10000 || updatedWallet.Version != 1 {
		t.Fatalf(
			"recebi saldo %d e versão %d",
			updatedWallet.BalanceCents,
			updatedWallet.Version,
		)
	}

	var storedState string
	var storedBalance int64
	err = pool.QueryRow(ctx, `
		SELECT state, balance_after_cents
		FROM wager_transactions
		WHERE id = $1
	`, wager.ID).Scan(&storedState, &storedBalance)
	if err != nil {
		t.Fatal(err)
	}

	if storedState != "REJECTED" || storedBalance != 10000 {
		t.Fatalf("resultado inesperado no banco: estado=%s saldo=%d", storedState, storedBalance)
	}

	var debitCount int64
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM wallet_ledger_entries
		WHERE wallet_id = $1
		  AND transaction_id = $2
		  AND direction = 'DEBIT'
	`, wallet.ID, wager.ID).Scan(&debitCount)
	if err != nil {
		t.Fatal(err)
	}

	if debitCount != 0 {
		t.Fatalf("esperava um lançamento DEBIT, recebi %d", debitCount)
	}

	t.Logf("walletId=%s transactionId=%s", wallet.ID, wager.ID)
}
