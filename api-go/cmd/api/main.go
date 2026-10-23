// Comando api: o lado Go da migracao do Loobby.
//
// Durante o strangler este processo roda ao lado do Kotlin, atras de um proxy que
// direciona rota a rota. Os dois compartilham banco e JWT_SECRET, entao a
// migracao e reversivel por rota. Ordem prevista:
//
//	legal -> games -> files -> notifications -> groups -> users -> events -> auth
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/games"
	"github.com/JamesWillian/loobby-api/api-go/internal/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(); err != nil {
		log.Error("encerrando por erro", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Valores do bloco rawg: do application.yaml do Kotlin.
	rawg := games.NewRawgClient(games.RawgConfig{
		APIKey:           cfg.RawgAPIKey,
		BaseURL:          "https://api.rawg.io/api",
		ConnectTimeout:   3 * time.Second,
		ReadTimeout:      5 * time.Second,
		SearchPageSize:   20,
		FailureThreshold: 5,
		OpenFor:          2 * time.Minute,
	})
	gamesHandler := games.NewHandler(games.NewService(rawg, games.NewStore(pool)))

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           logging(server.Handler(server.Deps{Config: cfg, DB: pool, Games: gamesHandler})),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	if cfg.TLSEnabled() {
		tlsCfg, err := server.TLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSClientCAFile)
		if err != nil {
			return err
		}
		srv.TLSConfig = tlsCfg
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("servidor no ar", "endereco", srv.Addr, "tls_mutuo", cfg.TLSEnabled(),
			"aceita_refresh_como_access", cfg.AcceptRefreshTokenAsAccess)
		var err error
		if cfg.TLSEnabled() {
			err = srv.ListenAndServeTLS("", "") // certificados ja estao no TLSConfig
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("sinal recebido, drenando requisicoes em voo")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("req", "metodo", r.Method, "rota", r.URL.Path,
			"status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}
