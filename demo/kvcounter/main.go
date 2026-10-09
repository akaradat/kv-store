// Command kvcounter runs step 3 of the counter test (verify.md §2) against a live node or router: several
// clients add 1 to an already seeded key at the same time with GET → PUT ?ifVersion=v, retrying on 409.
// Seeding and verifying are done by hand with curl.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	c := newCounter(parseFlags())
	log := report{out: os.Stdout}
	log.intro(c)

	clients, err := c.concurrentIncrease(ctx)
	must(err)
	log.increased(c, clients)
}

func parseFlags() config {
	url := flag.String("url", "http://localhost:7000", "base URL of a node or router")
	key := flag.String("key", "counter-with-ifversion", "key to increment; seed it with {\"count\":0} first")
	clients := flag.Int("clients", 3, "concurrent clients")
	increments := flag.Int("increments", 100, "successful increments per client")
	flag.Parse()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = *clients
	return config{
		url:        *url,
		key:        *key,
		clients:    *clients,
		increments: *increments,
		http:       &http.Client{Transport: transport, Timeout: 10 * time.Second},
	}
}

func must(err error) {
	if err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
}
