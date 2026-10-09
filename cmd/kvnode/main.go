package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/akaradat/kv-store/internal/core/cluster"
	"github.com/akaradat/kv-store/internal/core/store"
	"github.com/akaradat/kv-store/internal/transport/node"
)

const shutdownGrace = 5 * time.Second

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("kvnode stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := flag.String("addr", envOr("KV_ADDR", ":7000"), "listen address (env KV_ADDR)")
	nodeID := flag.String("node-id", envOr("KV_NODE_ID", "node-1"), "node ID reported by GET /kv (env KV_NODE_ID)")
	flag.Parse()

	if err := cluster.ValidateNodeID(*nodeID); err != nil {
		return fmt.Errorf("invalid -node-id %q: %w", *nodeID, err)
	}

	handler := node.NewHandler(node.NewHandlerInput{
		Store:  store.New(),
		NodeID: *nodeID,
		Log:    log,
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// No WriteTimeout: a GET /kv stream can legitimately run long (S7).
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	log.Info("kvnode listening", "addr", *addr, "node", *nodeID)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOr(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	return fallback
}
