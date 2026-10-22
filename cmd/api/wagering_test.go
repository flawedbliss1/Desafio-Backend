package main

import (
	"testing"
)

func TestHashWagerPayload(t *testing.T) {
	basereq := CreateWagerTransactionRequest{
		ProviderID:            "provider-teste",
		ExternalTransactionID: "aposta-001",
		PlayerID:              "11111111-1111-4111-8111-111111111111",
		WalletID:              "22222222-2222-4222-8222-222222222222",
		RoundID:               "rodada-001",
		GameID:                "jogo-001",
		Kind:                  "BET",
		Money: MoneyRequest{
			Amount:   "20.00",
			Currency: "BRL",
		},
	}
	texto := basereq.Money.Amount
	amountCents, err := parseAmountCents(texto)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := hashWagerPayload(basereq, amountCents)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(hash)

}
