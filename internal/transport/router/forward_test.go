package router_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/cluster"
	"github.com/akaradat/kv-store/internal/servertest"
)

var _ = Describe("key requests when the owning node misbehaves", func() {
	const timeout = 200 * time.Millisecond

	route := func(nodeURL string) string {
		_, url := servertest.StartRouter([]cluster.Node{{ID: "node-1", URL: nodeURL}}, timeout)
		return url
	}

	It("answers 504 when the node does not reply in time", Label("R-MN-FAIL-3"), func() {
		url := route(startFake(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		}))
		start := time.Now()
		r := send(url, "PUT", "/kv/a", `{}`)
		Expect(r.status).To(Equal(504))
		Expect(r.body).To(ContainSubstring(`"node_timeout"`))
		Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
	})

	It("answers 504 and does not retry when the connection drops after the request was sent", Label("R-MN-FAIL-2"), func() {
		var calls atomic.Int32
		url := route(startFake(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			hijackAndClose(w)
		}))
		Expect(send(url, "PUT", "/kv/a", `{}`).status).To(Equal(504))
		Expect(send(url, "PATCH", "/kv/a", `{}`).status).To(Equal(504))
		Expect(calls.Load()).To(BeEquivalentTo(2))
	})

	It("answers 504 when the reply body is cut off", Label("R-MN-FAIL-2"), func() {
		url := route(startFake(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"key":"a"`))
			http.NewResponseController(w).Flush()
			hijackAndClose(w)
		}))
		Expect(send(url, "GET", "/kv/a", ``).status).To(Equal(504))
	})

	It("answers 502 for a status a node never sends", Label("R-MN-FAIL-2"), func() {
		url := route(startFake(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		r := send(url, "GET", "/kv/a", ``)
		Expect(r.status).To(Equal(502))
		Expect(r.body).To(ContainSubstring(`"bad_node_response"`))
	})

	It("answers 502 for a reply that is not HTTP", Label("R-MN-FAIL-2"), func() {
		Expect(send(route(startNonHTTPServer()), "GET", "/kv/a", ``).status).To(Equal(502))
	})

	It("does not slow down keys on other nodes", Label("R-MN-CON-2", "R-MN-FAIL-1"), func() {
		healthy := servertest.StartNode("node-1")
		slow := startFake(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		})
		topology, url := servertest.StartRouter([]cluster.Node{{ID: "node-1", URL: healthy.URL}, {ID: "node-2", URL: slow}}, 5*time.Second)
		var slowKey, fastKey string
		for i := 0; slowKey == "" || fastKey == ""; i++ {
			k := fmt.Sprintf("k-%d", i)
			if topology.Owner(cluster.Slot(k)).ID == "node-2" {
				slowKey = k
			} else {
				fastKey = k
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		stuck := make(chan struct{})
		go func() {
			defer close(stuck)
			req, _ := http.NewRequestWithContext(ctx, "GET", url+"/kv/"+slowKey, nil)
			if resp, err := http.DefaultClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}()
		DeferCleanup(func() {
			cancel()
			<-stuck
		})
		Consistently(stuck, 100*time.Millisecond).ShouldNot(BeClosed())

		start := time.Now()
		Expect(send(url, "PUT", "/kv/"+fastKey, `{}`).status).To(Equal(200))
		Expect(time.Since(start)).To(BeNumerically("<", time.Second))
	})
})
