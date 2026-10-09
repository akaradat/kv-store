package router_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/cluster"
	"github.com/akaradat/kv-store/internal/servertest"
)

func headersOnlyThenDrop(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	http.NewResponseController(w).Flush()
	hijackAndClose(w)
}

var _ = Describe("GET /kv when a node fails after the 200 has been sent", func() {
	const timeout = 300 * time.Millisecond

	routeWithBrokenSecondNode := func(broken http.HandlerFunc) string {
		healthy := servertest.StartNode("node-1")
		for _, k := range []string{"a", "b", "c"} {
			send(healthy.URL, "PUT", "/kv/"+k, `{}`)
		}
		_, url := servertest.StartRouter([]cluster.Node{
			{ID: "node-1", URL: healthy.URL},
			{ID: "node-2", URL: startFake(broken)},
		}, timeout)
		return url
	}

	headersThen := func(rest func(w http.ResponseWriter)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"key":"x","node":"node-2"}` + "\n" + `{"key":"y","node":"node-2"}` + "\n"))
			http.NewResponseController(w).Flush()
			rest(w)
		}
	}

	It("still sends the 200 when the first node breaks before sending anything", Label("R-MN-LIST-6"), func() {
		broken := startFake(headersOnlyThenDrop)
		healthy := servertest.StartNode("node-2")
		_, url := servertest.StartRouter([]cluster.Node{{ID: "node-1", URL: broken}, {ID: "node-2", URL: healthy.URL}}, timeout)

		resp, body, err := readListUntilError(url)
		Expect(resp.StatusCode).To(Equal(200))
		Expect(err).To(HaveOccurred())
		Expect(body).To(BeEmpty())
	})

	DescribeTable("ends the body without a final newline and breaks the connection",
		func(broken http.HandlerFunc) {
			resp, body, err := readListUntilError(routeWithBrokenSecondNode(broken))
			Expect(resp.StatusCode).To(Equal(200))
			Expect(err).To(HaveOccurred(), "the client must see the transfer break")
			Expect(body).NotTo(HaveSuffix("\n"))
			Expect(body).To(ContainSubstring(`"node":"node-1"`))
		},
		Entry("the connection drops", Label("R-MN-LIST-6"), headersThen(func(w http.ResponseWriter) {
			hijackAndClose(w)
		})),
		Entry("the node goes quiet for longer than the node timeout", Label("R-MN-LIST-6", "R-MN-FAIL-3"), headersThen(func(w http.ResponseWriter) {
			time.Sleep(3 * timeout)
		})),
		Entry("the node's stream ends mid-line", Label("R-MN-LIST-6"), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"key":"x","node":"no`))
		}),
	)
})
