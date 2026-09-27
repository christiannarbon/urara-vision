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

	"urara-vision/backend/internal/chat/agent"
	"urara-vision/backend/internal/chat/apiclient"
	"urara-vision/backend/internal/chat/config"
	"urara-vision/backend/internal/chat/httpapi"
	"urara-vision/backend/internal/chat/llm"
	"urara-vision/backend/internal/chat/llm/anthropic"
	"urara-vision/backend/internal/chat/llm/gemini"
	"urara-vision/backend/internal/chat/logging"
)

// registerProviders is the one place adapters are wired in.
func registerProviders() {
	llm.Register("vertex", gemini.New)
	llm.Register("vertex-anthropic", anthropic.New)
}

func main() {
	os.Exit(run())
}

func run() int {
	registerProviders()
	cfg, err := config.Load(llm.Names())
	if err != nil {
		logging.New("error").Error("configuration is invalid, refusing to start: " + err.Error())
		return 1
	}
	log := logging.New(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting, backend at " + cfg.BackendBaseURL)

	// Built once here: a model per request would add latency to every turn.
	model, err := llm.New(context.Background(), *cfg, log)
	if err != nil {
		log.Error("language model configuration is invalid, refusing to start: " + err.Error())
		return 1
	}
	log.Info("language model configured", "llm", llm.Describe(*cfg))
	log.Info("conversation turns are serialised per process, not across replicas; "+
		"run one replica or expect interleaved transcripts",
		"max_concurrent_turns", cfg.MaxConcurrentTurns)

	backend := apiclient.New(cfg.BackendBaseURL, cfg.BackendAPIToken, cfg.BackendTimeout)
	cards := agent.NewCardCache(cfg.ContextCacheTTL, agent.MaxCachedCards, nil)
	answerer := agent.New(model, backend, cards, agent.Options{
		ModelName:         cfg.LLMModel,
		MaxHistory:        cfg.MaxHistoryMessages,
		MaxToolIterations: cfg.MaxToolIterations,
		MaxTurnTokens:     cfg.MaxTurnTokens,
	}, log)
	handler := httpapi.New(httpapi.Deps{
		Settings: cfg,
		Log:      log,
		Backend:  backend,
		Model:    model,
		Tools:    backend,
		Agent:    answerer,
	}).Handler()
	srv := &http.Server{
		Addr:              cfg.AppAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// No WriteTimeout: a turn may take ANSWER_TIMEOUT_SECONDS.
		IdleTimeout: 2 * time.Minute,
	}

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
		stop() // a second signal now kills the process
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
