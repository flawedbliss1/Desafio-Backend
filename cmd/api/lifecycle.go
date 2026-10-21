package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

func registerPoolShutdown(
	lc fx.Lifecycle,
	pool *pgxpool.Pool,
) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			log.Println("Fechando pool PostgreSQL")
			pool.Close()
			return nil
		},
	})
}

func registerHTTPServer(
	lc fx.Lifecycle,
	mux *http.ServeMux,
	shutdowner fx.Shutdowner,
) {
	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: mux,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			listener, err := net.Listen("tcp", server.Addr)
			if err != nil {
				return err
			}
			go func() {
				err := server.Serve(listener)
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Println("Falha no servidor HTTP:", err)

					err = shutdowner.Shutdown(fx.ExitCode(1))
					if err != nil {
						log.Println("Falha ao solicitar encerramento:", err)
					}
				}
			}()

			log.Println("API iniciada em http://" + server.Addr)
			return nil
		},

		OnStop: func(ctx context.Context) error {
			log.Println("Encerrando servidor HTTP")

			err := server.Shutdown(ctx)
			if err != nil {
				_ = server.Close()
			}
			return err
		},
	})
}
