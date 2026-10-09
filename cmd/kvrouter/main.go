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
	"github.com/akaradat/kv-store/internal/transport/router"
)

const shutdownGrace = 5 * time.Second

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(log); err != nil {
		log.Error("kvrouter stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := flag.String("addr", ":7000", "listen address for clients")
	clusterPath := flag.String("cluster", "", "path to cluster.json (required)")
	nodeTimeout := flag.Duration("node-timeout", 2*time.Second, "per-request timeout to a node")
	flag.Parse()

	if *clusterPath == "" {
		return errors.New("-cluster is required")
	}
	if *nodeTimeout <= 0 {
		return fmt.Errorf("-node-timeout must be positive, got %s", *nodeTimeout)
	}
	data, err := os.ReadFile(*clusterPath)
	if err != nil {
		return err
	}
	topology, err := cluster.Parse(data)
	if err != nil {
		return err
	}

	handler := router.NewHandler(router.NewHandlerInput{
		Cluster:     topology,
		NodeTimeout: *nodeTimeout,
		Log:         log,
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
	for _, n := range topology.Nodes() {
		log.Info("node", "id", n.ID, "url", n.URL)
	}
	log.Info("kvrouter listening", "addr", *addr, "nodes", len(topology.Nodes()), "node_timeout", *nodeTimeout)

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
