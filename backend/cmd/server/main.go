// Command server runs the urara-vision HTTP API.
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

	"urara-vision/backend/internal/api"
	"urara-vision/backend/internal/config"
	neostore "urara-vision/backend/internal/store/neo4j"
	"urara-vision/backend/internal/store/postgres"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Both datastores start alongside this process under compose and
	// Kubernetes, so retry rather than crash-looping on a cold start.
	pg, err := postgres.New(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pg.Close()

	if err := waitFor(ctx, log, "postgres", pg.Ping); err != nil {
		return err
	}
	if err := pg.Migrate(ctx); err != nil {
		return err
	}
	log.Info("postgres ready, schema applied")

	neo, err := neostore.New(cfg.Neo4jURI, cfg.Neo4jUser, cfg.Neo4jPassword)
	if err != nil {
		return err
	}
	defer func() { _ = neo.Close(context.Background()) }()

	if err := waitFor(ctx, log, "neo4j", neo.Ping); err != nil {
		return err
	}
	if err := neo.EnsureConstraints(ctx); err != nil {
		return err
	}
	log.Info("neo4j ready, constraints ensured")

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.New(cfg, pg, neo, log).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		// Ingest bodies can be large and slow to upload; keep the write side
		// generous but bounded.
		WriteTimeout: 3 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}

	if cfg.APIToken == "" {
		log.Warn("API_TOKEN is not set: /api/v1 is open to anyone who can reach this port")
	} else {
		log.Info("API token authentication enabled")
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// waitFor retries a health check with backoff until it succeeds or the context
// is cancelled.
func waitFor(ctx context.Context, log *slog.Logger, name string, ping func(context.Context) error) error {
	const maxAttempts = 40
	delay := 250 * time.Millisecond

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := ping(checkCtx)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Warn("waiting for dependency", "dependency", name, "attempt", attempt, "error", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		if delay < 3*time.Second {
			delay *= 2
		}
	}
	return errors.New(name + " did not become ready in time")
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
