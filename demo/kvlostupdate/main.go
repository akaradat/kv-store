// Command kvlostupdate is the negative control for kvcounter (verify.md §2): the same concurrent increments on
// an already seeded key, but every PUT leaves out ?ifVersion, so clients overwrite each other's increments.
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

	must(c.concurrentIncrease(ctx))
	log.increased(c)
}

func parseFlags() config {
	url := flag.String("url", "http://localhost:7000", "base URL of a node or router")
	key := flag.String("key", "counter-without-ifversion", "key to increment; seed it with {\"count\":0} first")
	clients := flag.Int("clients", 3, "concurrent clients")
	increments := flag.Int("increments", 100, "writes per client")
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
