package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func writeWagerError(w http.ResponseWriter, status int, code string) {

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})

}

func registerWagerRoutes(mux *http.ServeMux, pool *pgxpool.Pool, httpClient *http.Client, authConfig AuthConfig) {

	handler := func(w http.ResponseWriter, r *http.Request, providerID string) {
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if strings.TrimSpace(idempotencyKey) == "" {
			writeWagerError(w, http.StatusBadRequest, "missing_idempotency_key")
			return
		}
		var req CreateWagerTransactionRequest

		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&req); err != nil {
			writeWagerError(w, http.StatusBadRequest, "invalid_json")
			return
		}

		if strings.TrimSpace(req.ProviderID) == "" ||
			strings.TrimSpace(req.ExternalTransactionID) == "" ||
			strings.TrimSpace(req.RoundID) == "" ||
			strings.TrimSpace(req.GameID) == "" {
			writeWagerError(
				w,
				http.StatusBadRequest,
				"missing_required_fields",
			)
			return
		}

		if req.ProviderID != providerID {
			writeWagerError(
				w,
				http.StatusForbidden,
				"provider_mismatch",
			)
			return
		}
		if req.Kind != "BET" {
			writeWagerError(w, http.StatusBadRequest, "unsupported_kind")
			return
		}

		if req.Money.Currency != "BRL" {
			writeWagerError(
				w,
				http.StatusBadRequest,
				"unsupported_currency",
			)
			return
		}

		playerID, err := uuid.Parse(req.PlayerID)
		if err != nil {
			writeWagerError(w, http.StatusBadRequest, "invalid_player_id")
			return
		}

		walletID, err := uuid.Parse(req.WalletID)
		if err != nil {
			writeWagerError(w, http.StatusBadRequest, "invalid_wallet_id")
			return
		}

		req.PlayerID = playerID.String()
		req.WalletID = walletID.String()

		amountCents, err := parseAmountCents(req.Money.Amount)
		if err != nil || amountCents <= 0 {
			writeWagerError(w, http.StatusBadRequest, "invalid_amount")
			return
		}

		payloadHash, err := hashWagerPayload(req, amountCents)
		if err != nil {
			log.Println("Erro ao gerar hash da BET:", err)
			writeWagerError(
				w,
				http.StatusInternalServerError,
				"internal_error",
			)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		wager, replay, err := processBet(
			ctx,
			pool,
			req,
			amountCents,
			idempotencyKey,
			payloadHash,
		)
		if err != nil {
			status := http.StatusInternalServerError
			code := "internal_error"

			switch {
			case errors.Is(err, ErrIdempotencyConflict):
				status, code = http.StatusConflict, "idempotency_conflict"

			case errors.Is(err, ErrExternalTransactionConflict):
				status, code = http.StatusConflict, "external_transaction_conflict"

			case errors.Is(err, ErrWalletNotFound):
				status, code = http.StatusNotFound, "wallet_not_found"

			case errors.Is(err, ErrWalletPlayerMismatch):
				status, code = http.StatusUnprocessableEntity, "wallet_player_mismatch"

			case errors.Is(err, ErrCurrencyMismatch):
				status, code = http.StatusUnprocessableEntity, "currency_mismatch"

			case errors.Is(err, ErrWagerResultUnavailable):
				status, code = http.StatusServiceUnavailable, "wager_result_unavailable"

			case errors.Is(err, context.DeadlineExceeded),
				errors.Is(err, context.Canceled):
				status, code = http.StatusServiceUnavailable, "processing_unavailable"

			default:
				log.Println("Erro ao processar BET:", err)
			}

			writeWagerError(w, status, code)
			return
		}

		if wager.BalanceAfterCents == nil {
			log.Println("BET retornou resultado sem saldo:", wager.ID)
			writeWagerError(
				w,
				http.StatusInternalServerError,
				"internal_error",
			)
			return
		}

		// Usa o saldo registrado no resultado original da transação.
		balanceCents := *wager.BalanceAfterCents

		response := map[string]any{
			"transactionId": wager.ID,
			"status":        wager.State,
			"balance": map[string]string{
				"amount": fmt.Sprintf(
					"%d.%02d",
					balanceCents/100,
					balanceCents%100,
				),
				"currency": wager.Currency,
			},
			"idempotentReplay": replay,
		}

		if wager.FailureCode != nil {
			response["failureCode"] = *wager.FailureCode
		}

		status := http.StatusOK
		if wager.State == "REJECTED" {
			status = http.StatusUnprocessableEntity
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(response)
	}

	mux.HandleFunc(
		"POST /wagering/transactions",
		requireProviderRole(
			httpClient,
			authConfig,
			"wager:write",
			handler,
		),
	)
}
