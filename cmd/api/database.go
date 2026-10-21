package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

func newPool(lc fx.Lifecycle) (*pgxpool.Pool, error) {

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL não foi definida")
	}
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
	log.Print("Conexão com PostgreSQL confirmada")
	log.Println("Configuração carregada")

	registerPoolShutdown(lc, pool)

	return pool, nil

}
