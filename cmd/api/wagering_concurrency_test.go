package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProcessBetConcurrentBalance(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Faltando TEST_DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var playerID string
	err = pool.QueryRow(
		ctx,
		"SELECT gen_random_uuid()::text",
	).Scan(&playerID)
	if err != nil {
		t.Fatal(err)
	}

	wallet, err := createWallet(ctx, pool, playerID, "BRL", 10000)
	if err != nil {
		t.Fatalf("criar carteira: %v", err)
	}

	amountCents, err := parseAmountCents("80.00")
	if err != nil {
		t.Fatal(err)
	}

	type betResult struct {
		wager  StoredWager
		replay bool
		err    error
	}

	start := make(chan struct{})

	results := make(chan betResult, 2)

	for i := 1; i <= 2; i++ {
		req := CreateWagerTransactionRequest{
			ProviderID: "provider-teste",
			ExternalTransactionID: fmt.Sprintf(
				"bet-%s-%d",
				wallet.ID,
				i,
			),
			PlayerID: playerID,
			WalletID: wallet.ID,
			RoundID:  "round-teste",
			GameID:   "game-teste",
			Kind:     "BET",
			Money: MoneyRequest{
				Amount:   "80.00",
				Currency: "BRL",
			},
		}

		key := fmt.Sprintf("key-%s-%d", wallet.ID, i)

		go func(req CreateWagerTransactionRequest, key string) {
			<-start

			payloadHash, err := hashWagerPayload(req, amountCents)
			if err != nil {
				results <- betResult{err: err}
				return
			}

			wager, replay, err := processBet(
				ctx,
				pool,
				req,
				amountCents,
				key,
				payloadHash,
			)

			results <- betResult{
				wager:  wager,
				replay: replay,
				err:    err,
			}
		}(req, key)
	}

	close(start)

	received := []betResult{<-results, <-results}

	approved := 0
	rejected := 0

	for _, result := range received {
		if result.err != nil {
			t.Fatalf("processar BET concorrente: %v", result.err)
		}

		if result.replay {
			t.Fatal("replay gerado por apostas diferentes")
		}

		if result.wager.State == "PROCESSED" {
			approved++
		} else if result.wager.State == "REJECTED" {
			rejected++
		} else {
			t.Fatalf("estado inesperado: %s", result.wager.State)
		}

		if result.wager.BalanceAfterCents == nil ||
			*result.wager.BalanceAfterCents != 2000 {
			t.Fatal("esperava saldo resultante de 2000 centavos")
		}
	}

	if approved != 1 || rejected != 1 {
		t.Fatalf(
			"esperava uma aprovada e uma rejeitada; recebi %d e %d",
			approved,
			rejected,
		)
	}

	updatedWallet, err := findWallet(ctx, pool, wallet.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updatedWallet.BalanceCents != 2000 ||
		updatedWallet.Version != 2 {
		t.Fatalf(
			"recebi saldo %d e versão %d",
			updatedWallet.BalanceCents,
			updatedWallet.Version,
		)
	}

	var debitCount int64
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM wallet_ledger_entries
		WHERE wallet_id = $1
		  AND direction = 'DEBIT'
	`, wallet.ID).Scan(&debitCount)
	if err != nil {
		t.Fatal(err)
	}

	if debitCount != 1 {
		t.Fatalf("esperava um DEBIT, recebi %d", debitCount)
	}

	t.Logf(
		"aprovadas=%d rejeitadas=%d saldo=%d versão=%d débitos=%d",
		approved,
		rejected,
		updatedWallet.BalanceCents,
		updatedWallet.Version,
		debitCount,
	)
}

func TestProcessBetConcurrentReplay(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Faltando TEST_DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var playerID string
	err = pool.QueryRow(
		ctx,
		"SELECT gen_random_uuid()::text",
	).Scan(&playerID)
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

	key := "key-" + wallet.ID

	type betResult struct {
		wager  StoredWager
		replay bool
		err    error
	}

	const copies = 50
	start := make(chan struct{})
	results := make(chan betResult, copies)

	for i := 0; i < copies; i++ {
		go func() {
			<-start

			wager, replay, err := processBet(
				ctx,
				pool,
				req,
				amountCents,
				key,
				payloadHash,
			)

			results <- betResult{
				wager:  wager,
				replay: replay,
				err:    err,
			}
		}()
	}

	close(start)

	received := make([]betResult, copies)
	for i := range received {
		received[i] = <-results
	}

	transactionID := received[0].wager.ID
	originalCount := 0
	replayCount := 0

	for _, result := range received {
		if result.err != nil {
			t.Fatalf("processar aposta repetida: %v", result.err)
		}

		if result.wager.ID != transactionID {
			t.Fatalf(
				"esperava transação %s, recebi %s",
				transactionID,
				result.wager.ID,
			)
		}

		if result.wager.State != "PROCESSED" {
			t.Fatalf("estado inesperado: %s", result.wager.State)
		}

		if result.wager.BalanceAfterCents == nil ||
			*result.wager.BalanceAfterCents != 9000 {
			t.Fatal("esperava saldo de 9000")
		}

		if result.replay {
			replayCount++
		} else {
			originalCount++
		}
	}

	if originalCount != 1 || replayCount != copies-1 {
		t.Fatalf(
			"esperava 1 execução nova e 49 replays; recebi %d e %d",
			originalCount,
			replayCount,
		)
	}

	updatedWallet, err := findWallet(ctx, pool, wallet.ID)
	if err != nil {
		t.Fatal(err)
	}

	if updatedWallet.BalanceCents != 9000 ||
		updatedWallet.Version != 2 {
		t.Fatalf(
			"recebi saldo %d e versão %d",
			updatedWallet.BalanceCents,
			updatedWallet.Version,
		)
	}

	var betCount int64
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM wager_transactions
		WHERE wallet_id = $1
		  AND kind = 'BET'
	`, wallet.ID).Scan(&betCount)
	if err != nil {
		t.Fatal(err)
	}

	if betCount != 1 {
		t.Fatalf("esperava uma BET registrada, recebi %d", betCount)
	}

	var debitCount int64
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM wallet_ledger_entries
		WHERE wallet_id = $1
		  AND direction = 'DEBIT'
	`, wallet.ID).Scan(&debitCount)
	if err != nil {
		t.Fatal(err)
	}

	if debitCount != 1 {
		t.Fatalf("esperava um DEBIT, recebi %d", debitCount)
	}

	t.Logf(
		"execuções novas=%d replays=%d saldo=%d BETs=%d débitos=%d",
		originalCount,
		replayCount,
		updatedWallet.BalanceCents,
		betCount,
		debitCount,
	)
}
