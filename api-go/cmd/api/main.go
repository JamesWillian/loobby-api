// Comando api: a API do Loobby em Go. Comecou como porta completa da API
// Kotlin (arquivada na tag arquivo/v1) e e o unico backend desde 16/11/2026.
// Todos os modulos estao aqui: legal, games, files, notifications, groups,
// users, events, teams, auth e appconfig.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JamesWillian/loobby-api/api-go/internal/auth"
	"github.com/JamesWillian/loobby-api/api-go/internal/config"
	"github.com/JamesWillian/loobby-api/api-go/internal/db"
	"github.com/JamesWillian/loobby-api/api-go/internal/email"
	"github.com/JamesWillian/loobby-api/api-go/internal/events"
	"github.com/JamesWillian/loobby-api/api-go/internal/files"
	"github.com/JamesWillian/loobby-api/api-go/internal/games"
	"github.com/JamesWillian/loobby-api/api-go/internal/groups"
	"github.com/JamesWillian/loobby-api/api-go/internal/notifications"
	"github.com/JamesWillian/loobby-api/api-go/internal/server"
	"github.com/JamesWillian/loobby-api/api-go/internal/teams"
	"github.com/JamesWillian/loobby-api/api-go/internal/users"
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

	deps, err := build(ctx, cfg, pool)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           logging(server.Handler(deps)),
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

// build constroi os modulos e os liga uns aos outros.
func build(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) (server.Deps, error) {
	storage, err := files.NewStorage(cfg.FileStorageLocation)
	if err != nil {
		return server.Deps{}, fmt.Errorf("pasta de uploads: %w", err)
	}

	// Valores do bloco rawg: do application.yaml do Kotlin.
	rawg := games.NewRawgClient(games.RawgConfig{
		APIKey:           cfg.RawgAPIKey,
		BaseURL:          cfg.RawgBaseURL,
		ConnectTimeout:   3 * time.Second,
		ReadTimeout:      5 * time.Second,
		SearchPageSize:   20,
		FailureThreshold: 5,
		OpenFor:          2 * time.Minute,
	})
	gamesSvc := games.NewService(rawg, games.NewStore(pool))

	// Push: com NOTIFICATIONS_ENABLED (o padrao), as credenciais do Firebase sao
	// obrigatorias e a falta delas impede a partida, como no Kotlin.
	var sender notifications.Sender = notifications.DisabledSender{}
	if cfg.NotificationsEnabled {
		fcm, err := notifications.NewFCM(pool, cfg.FirebaseCredentialsPath)
		if err != nil {
			return server.Deps{}, err
		}
		sender = fcm
	} else {
		slog.Warn("notificacoes DESLIGADAS: nenhum push sera enviado")
	}
	notifier := notifications.NewService(pool, sender)

	// Os jobs so rodam com o envio ligado. (No Kotlin eles rodam mesmo com o
	// envio desligado, registrando como "enviado" o que nao enviaram; com dois
	// processos dividindo o banco, isso faria o lado desligado engolir os
	// avisos do lado ligado.)
	if cfg.NotificationsEnabled && cfg.NotificationSchedulerEnabled {
		scheduler, err := notifications.NewScheduler(pool, notifier)
		if err != nil {
			return server.Deps{}, err
		}
		go scheduler.Run(ctx)
	}

	tokens := auth.NewTokens(cfg.JWTSecret)
	authSvc := auth.NewService(pool, tokens,
		email.NewResend(cfg.ResendAPIKey, config.ResendFromEmail, cfg.ResendBaseURL),
		auth.NewGoogleVerifier(cfg.GoogleClientIDs, cfg.GoogleCertsURL),
		auth.Options{BaseURL: cfg.BaseURL, AllowUnsignedAnonRefresh: cfg.AllowUnsignedAnonRefresh})
	if cfg.AllowUnsignedAnonRefresh {
		slog.Warn("ALLOW_UNSIGNED_ANON_REFRESH ligado: refresh token de anonimo e aceito sem conferir assinatura")
	}

	return server.Deps{
		Config:  cfg,
		DB:      pool,
		Tokens:  tokens,
		Auth:    auth.NewHandler(authSvc),
		Users:   users.NewHandler(users.NewService(pool), storage),
		Groups:  groups.NewHandler(groups.NewService(pool, storage)),
		Events:  events.NewHandler(events.NewService(pool, notifier, gamesSvc)),
		Teams:   teams.NewHandler(teams.NewService(pool)),
		Games:   games.NewHandler(gamesSvc),
		Devices: notifications.NewHandler(pool),
		Files:   storage,
	}, nil
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
