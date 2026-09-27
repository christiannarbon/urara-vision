// Command chat runs the urara-vision chat service.
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

	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/chat/logging"
)

// Replaced by the provider registry in Phase 17.
var allowedProviders = []string{"vertex"}

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.Load(allowedProviders)
	if err != nil {
		logging.New("error").Error("configuration is invalid, refusing to start: " + err.Error())
		return 1
	}
	log := logging.New(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting, backend at " + cfg.BackendBaseURL)
	log.Info("conversation turns are serialised per process, not across replicas; "+
		"run one replica or expect interleaved transcripts",
		"max_concurrent_turns", cfg.MaxConcurrentTurns)

	handler := httpapi.New(httpapi.Deps{Settings: cfg, Log: log}).Handler()
	srv := &http.Server{Addr: cfg.AppAddr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.AppAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		log.Error("server failed", "error", err)
		return 1
	case <-ctx.Done():
		log.Info("shutting down")
	}

	// Long enough for a turn in flight to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.AnswerTimeout+10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown incomplete", "error", err)
		return 1
	}
	log.Info("stopped")
	return 0
}
