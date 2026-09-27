// Command strata-server renders strata SVGs for public GitHub repositories
// and stores them in Cloudflare Workers KV, where the strata Worker serves
// them. See deploy/README.md.
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

	"github.com/umuttopalak/strata/internal/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := server.ConfigFromEnv()
	if err != nil {
		return err
	}

	var store server.Store = server.DirStore{Dir: cfg.StoreDir}
	if cfg.CFAPIToken != "" {
		store = &server.KVStore{
			BaseURL:     "https://api.cloudflare.com/client/v4",
			AccountID:   cfg.CFAccountID,
			NamespaceID: cfg.CFNamespaceID,
			Token:       cfg.CFAPIToken,
		}
	}
	gh := &server.GitHub{BaseURL: "https://api.github.com", Token: cfg.GitHubToken}
	srv := server.New(cfg, gh, store, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go srv.Run(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdown)
	}()

	log.Info("listening", "addr", cfg.Addr, "kv", cfg.CFAPIToken != "", "max_repo_mb", cfg.MaxRepoMB)
	if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
