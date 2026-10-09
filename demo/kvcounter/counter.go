package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
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

type clientResult struct {
	ok        int
	conflicts int
}

// concurrentIncrease runs every client at once, waits for all, and stops all at the first error.
func (c counter) concurrentIncrease(ctx context.Context) ([]clientResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]clientResult, c.cfg.clients)
	var firstErr error
	var once sync.Once
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range c.cfg.clients {
		wg.Go(func() {
			<-start
			var err error
			results[i], err = c.increment(ctx)
			if err != nil {
				once.Do(func() { firstErr = fmt.Errorf("client %d: %w", i+1, err) })
				cancel()
			}
		})
	}
	// Released together so their requests really overlap.
	close(start)
	wg.Wait()
	return results, firstErr
}

// increment is one client: read the count and its version, write count+1 only if the version is unchanged,
// and on 409 (another client wrote first) read again.
func (c counter) increment(ctx context.Context) (clientResult, error) {
	var res clientResult
	for res.ok < c.cfg.increments {
		cur, err := c.get(ctx)
		if err != nil {
			return res, err
		}

		status, err := c.put(ctx, c.keyURL+"?ifVersion="+strconv.FormatUint(cur.Version, 10), cur.Value.Count+1)
		switch {
		case err != nil:
			return res, err
		case status == http.StatusOK:
			res.ok++
		case status == http.StatusConflict:
			res.conflicts++
		default:
			return res, fmt.Errorf("PUT: unexpected status %d", status)
		}
	}
	return res, nil
}

func totalConflicts(results []clientResult) int {
	total := 0
	for _, r := range results {
		total += r.conflicts
	}
	return total
}
