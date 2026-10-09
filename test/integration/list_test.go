package integration_test

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/cluster"
	"github.com/akaradat/kv-store/internal/servertest"
)

var _ = Describe("GET /kv through the router", func() {
	It("returns 200 with an empty body for an empty deployment", Label("R-MN-LIST-7"), func() {
		c := servertest.StartCluster(3)
		r := send(c.URL, "GET", "/kv", "")
		Expect(r.status).To(Equal(200))
		Expect(r.contentType).To(Equal("application/x-ndjson"))
		Expect(r.body).To(BeEmpty())
	})

	It("lists every key on every node once, with its owner", Label("R-MN-LIST-1", "R-MN-LIST-2", "R-MN-LIST-3"), func() {
		c := servertest.StartCluster(3)
		// Two keys on each node, so a router that skips any node loses keys and fails below.
		var keys []string
		for _, n := range c.Nodes {
			keys = append(keys, c.KeyOn(n, "a"), c.KeyOn(n, "b"))
		}
		for _, k := range keys {
			Expect(send(c.URL, "PUT", "/kv/"+k, `{}`).status).To(Equal(200))
		}

		r := send(c.URL, "GET", "/kv", "")
		Expect(r.status).To(Equal(200))
		Expect(r.body).To(HaveSuffix("\n"))

		// Expect: no key twice (R-MN-LIST-3), every written key and nothing else (R-MN-LIST-1), and each line
		// names the node that owns the key (R-MN-LIST-2).
		got := map[string]string{}
		for _, l := range parseLines(r.body) {
			Expect(got).NotTo(HaveKey(l.Key), "listed twice")
			got[l.Key] = l.Node
		}
		Expect(got).To(HaveLen(len(keys)))
		for _, k := range keys {
			Expect(got[k]).To(Equal(c.Owner(k).ID), k)
		}
	})

	It("spreads 30,000 keys written through the router evenly and lists each exactly once", Label("R-MN-TOP-3", "R-MN-TOP-5", "R-MN-LIST-3"), func() {
		c := servertest.StartCluster(3)
		// verify.md §4: 30,000 random keys, seeded so a failure can be replayed.
		const total = 30000
		rng := rand.New(rand.NewPCG(3, 4))
		keys := make(chan string, total)
		for range total {
			keys <- fmt.Sprintf("k-%016x", rng.Uint64())
		}
		close(keys)

		// Write them from 32 workers. Expect every PUT to answer 200.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConnsPerHost = 32
		DeferCleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport}
		failures := make(chan string, total)
		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				for k := range keys {
					req, err := http.NewRequest("PUT", c.URL+"/kv/"+k, strings.NewReader(`{}`))
					if err != nil {
						failures <- err.Error()
						continue
					}
					resp, err := client.Do(req)
					if err != nil {
						failures <- err.Error()
						continue
					}
					resp.Body.Close()
					if resp.StatusCode != 200 {
						failures <- k + ": " + resp.Status
					}
				}
			})
		}
		wg.Wait()
		close(failures)
		var failed []string
		for f := range failures {
			failed = append(failed, f)
		}
		Expect(failed).To(BeEmpty())

		// Expect the list to hold each key exactly once (R-MN-LIST-3), on the node the slot table names
		// (R-MN-TOP-3), and nothing else.
		r := send(c.URL, "GET", "/kv", "")
		Expect(r.body).To(HaveSuffix("\n"))
		counts := map[string]int{}
		seen := make(map[string]bool, total)
		for _, l := range parseLines(r.body) {
			Expect(seen[l.Key]).To(BeFalse(), "listed twice: %s", l.Key)
			seen[l.Key] = true
			Expect(l.Node).To(Equal(c.Topology.Owner(cluster.Slot(l.Key)).ID))
			counts[l.Node]++
		}
		Expect(seen).To(HaveLen(total))

		// Expect every node to own 9,000–11,000 keys, 90–110% of the 10,000 average (R-MN-TOP-5).
		for _, n := range c.Nodes {
			got := counts[n.ID]
			GinkgoWriter.Printf("%s: %d (%.1f%%)\n", n.ID, got, float64(got)*100/(total/3))
			Expect(got).To(BeNumerically(">=", 9000), n.ID)
			Expect(got).To(BeNumerically("<=", 11000), n.ID)
		}
	})

	It("answers 503 and no list when a node is down", Label("R-MN-LIST-6"), func() {
		c := servertest.StartCluster(3)
		// A key on a healthy node, so a router that returned a partial list would have something to send.
		Expect(send(c.URL, "PUT", "/kv/"+c.KeyOn(c.Nodes[0], "x"), `{}`).status).To(Equal(200))
		c.Nodes[2].Stop()

		// Expect a 503 error instead of a 200 list without node-3's keys.
		r := send(c.URL, "GET", "/kv", "")
		Expect(r.status).To(Equal(503))
		Expect(r.contentType).To(Equal("application/json"))
		Expect(r.body).To(MatchJSON(`{"error":"node_unavailable","message":"the owning node could not be reached; the request was not applied"}`))
	})

	It("lists all nodes again after a down node restarts", Label("R-MN-LIST-1", "R-MN-FAIL-4"), func() {
		c := servertest.StartCluster(3)
		n1, n3 := c.Nodes[0], c.Nodes[2]
		k1, k3 := c.KeyOn(n1, "x"), c.KeyOn(n3, "x")
		Expect(send(c.URL, "PUT", "/kv/"+k1, `{}`).status).To(Equal(200))
		Expect(send(c.URL, "PUT", "/kv/"+k3, `{}`).status).To(Equal(200))
		n3.Stop()
		n3.Restart()

		// Expect node-3 to be listed again but empty: only k1 (R-MN-FAIL-4).
		line1 := fmt.Sprintf(`{"key":%q,"node":%q}`, k1, n1.ID)
		Expect(lines(send(c.URL, "GET", "/kv", "").body)).To(ConsistOf(line1))

		// Expect a key written to node-3 after the restart to show up with node-3 as its owner (R-MN-LIST-1).
		Expect(send(c.URL, "PUT", "/kv/"+k3, `{}`).status).To(Equal(200))
		Expect(lines(send(c.URL, "GET", "/kv", "").body)).To(ConsistOf(
			line1,
			fmt.Sprintf(`{"key":%q,"node":%q}`, k3, n3.ID),
		))
	})
})
