package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func route(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("Database vazia")
	}
	log.Print("Conexão com PostgreSQL confirmada")

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal("Erro: ", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = pool.Ping(ctx)
	cancel()
	if err != nil {
		pool.Close()
		log.Fatal("Erro: ", err)
	}
	log.Println("Configuração carregada")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", route)
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		err := pool.Ping(ctx)
		cancel()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"status": "unavailable"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /wallets/{walletId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		walletId := r.PathValue("walletId")
		parsedID, err := uuid.Parse(walletId)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_wallet_id"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		wallet, err := findWallet(ctx, pool, parsedID.String())
		cancel()
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"error": "wallet_not_found"})
			return
		}
		if err != nil {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal_error"})
			return
		}
		casa1 := wallet.BalanceCents / 100
		casa2 := wallet.BalanceCents % 100
		walletF := fmt.Sprintf("%d.%02d", casa1, casa2)
		json.NewEncoder(w).Encode(map[string]any{
			"id":       wallet.ID,
			"playerId": wallet.PlayerID,
			"version":  wallet.Version,
			"balance": map[string]string{
				"amount":   walletF,
				"currency": wallet.Currency,
			},
		})
	})

	mux.HandleFunc("POST /wallets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req CreateWalletRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_json"})
			return
		}

		parsedPlayer, err := uuid.Parse(req.PlayerID)
		if err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_player_id"})
			return
		}

		currency := req.InitialBalance.Currency
		if currency != "BRL" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_currency"})
			return
		}

		initialCents, err := parseAmountCents(req.InitialBalance.Amount)
		if err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_amount"})
			return
		}
		if initialCents != 0 {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_initial_balance"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		wallet, err := createWallet(ctx, pool, parsedPlayer.String(), currency)
		cancel()
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				log.Println(pgErr.Code, pgErr.ConstraintName)
				if pgErr.Code == "23505" && pgErr.ConstraintName == "wallets_player_id_currency_key" {
					w.WriteHeader(409)
					json.NewEncoder(w).Encode(map[string]string{"error": "wallet_already_exists"})
					return
				}
			}
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal_error"})
			return
		}
		casa1 := wallet.BalanceCents / 100
		casa2 := wallet.BalanceCents % 100
		walletF := fmt.Sprintf("%d.%02d", casa1, casa2)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":       wallet.ID,
			"playerId": wallet.PlayerID,
			"version":  wallet.Version,
			"balance": map[string]string{
				"amount":   walletF,
				"currency": wallet.Currency,
			},
		})
	})

	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}

	log.Println("Iniciando API em http://127.0.0.1:8080")
	err = server.ListenAndServe()
	pool.Close()
	if err != nil {
		log.Fatal(err)
	}
}
