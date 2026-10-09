package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type config struct {
	url        string
	key        string
	clients    int
	increments int
	http       *http.Client
}

type counter struct {
	cfg    config
	keyURL string
}

func newCounter(cfg config) counter {
	return counter{cfg: cfg, keyURL: strings.TrimSuffix(cfg.url, "/") + "/kv/" + cfg.key}
}

// concurrentIncrease runs every client at once, waits for all, and stops all at the first error.
func (c counter) concurrentIncrease(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var firstErr error
	var once sync.Once
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range c.cfg.clients {
		wg.Go(func() {
			<-start
			if err := c.increment(ctx); err != nil {
				once.Do(func() { firstErr = fmt.Errorf("client %d: %w", i+1, err) })
				cancel()
			}
		})
	}
	// Released together so their requests really overlap.
	close(start)
	wg.Wait()
	return firstErr
}

// increment is one client: read the count and write count+1 with no ifVersion, so a write that lands between
// this client's GET and PUT is silently overwritten.
func (c counter) increment(ctx context.Context) error {
	for range c.cfg.increments {
		cur, err := c.get(ctx)
		if err != nil {
			return err
		}

		status, err := c.put(ctx, c.keyURL, cur.Value.Count+1)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("PUT: unexpected status %d", status)
		}
	}
	return nil
}
