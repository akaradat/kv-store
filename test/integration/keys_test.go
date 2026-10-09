package integration_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/servertest"
)

type step struct{ method, path, body string }

var singleNodeScript = []step{
	{"PUT", "/kv/user:42", `{"name":"Ari","points":10}`},
	{"GET", "/kv/user:42", ``},
	{"GET", "/kv/nope", ``},
	{"PUT", "/kv/user:42", `{"name":"Ari"}`},
	{"PUT", "/kv/user:42?ifVersion=2", `{"name":"Ari","points":20}`},
	{"PUT", "/kv/user:42?ifVersion=2", `{"name":"Ari","points":99}`},
	{"PATCH", "/kv/user:42", `{"rank":"gold","points":30}`},
	{"PATCH", "/kv/user:42?ifVersion=3", `{"rank":"silver"}`},
	{"PATCH", "/kv/user:43", `{"name":"Bo"}`},
	{"PUT", "/kv/user:42", `[1,2]`},
	{"PUT", "/kv/user:42", `null`},
	{"PUT", "/kv/user:42", `{bad`},
	{"PUT", "/kv/user:42?ifVersion=0", `{}`},
	{"PUT", "/kv/new-key?ifVersion=1", `{}`},
	{"GET", "/kv/new-key", ``},
	{"PUT", "/kv/bad%20key", `{}`},
	{"GET", "/kv/user%3A42", ``},
	{"DELETE", "/kv/user:42", ``},
	{"POST", "/kv", ``},
	{"GET", "/kv/a/b", ``},
	{"GET", "/", ``},
	{"GET", "/kv/", ``},
	{"PUT", "/kv/a?ifVersion=9", `[1]`},
	{"PUT", "/kv/big", `{"n":12345678901234567890,"f":1.0,"s":"<A&>","d":1,"d":2}`},
	{"GET", "/kv/big", ``},
	{"PUT", "/kv/a", `{"a":"` + strings.Repeat("x", 1<<20) + `"}`},
	{"GET", "/kv/user:42", ``},
}

var _ = Describe("key requests through the router", func() {
	It("answer with the same status, headers and body as a single node", Label("R-MN-API-1", "R-MN-API-2", "R-MN-API-4", "R-MN-TOP-6"), func() {
		single := servertest.StartNode("node-1")
		multi := servertest.StartCluster(3)
		solo := servertest.StartCluster(1)

		for _, s := range singleNodeScript {
			want := send(single.URL, s.method, s.path, s.body)
			Expect(send(multi.URL, s.method, s.path, s.body)).To(Equal(want), "3 nodes: %s %s", s.method, s.path)
			Expect(send(solo.URL, s.method, s.path, s.body)).To(Equal(want), "1 node: %s %s", s.method, s.path)
		}
	})

	It("store each key only on its owner", Label("R-MN-TOP-3"), func() {
		c := servertest.StartCluster(3)
		for _, key := range []string{"a", "b", "c", "user:42", "counter"} {
			Expect(send(c.URL, "PUT", "/kv/"+key, `{}`).status).To(Equal(200))
			for _, n := range c.Nodes {
				want := 404
				if n == c.Owner(key) {
					want = 200
				}
				Expect(send(n.URL, "GET", "/kv/"+key, "").status).To(Equal(want), "%s on %s", key, n.ID)
			}
		}
	})

	Describe("when the owning node is down", func() {
		var c *servertest.Cluster
		var down *servertest.Node
		var keyDown, keyUp string

		BeforeEach(func() {
			c = servertest.StartCluster(3)
			down = c.Nodes[1]
			keyDown = c.KeyOn(down, "down")
			keyUp = c.KeyOn(c.Nodes[0], "up")
			Expect(send(c.URL, "PUT", "/kv/"+keyDown, `{"x":1}`).status).To(Equal(200))
			Expect(send(c.URL, "PUT", "/kv/"+keyUp, `{"x":1}`).status).To(Equal(200))
			down.Stop()
		})

		It("answers 503 node_unavailable for its keys", Label("R-MN-FAIL-1"), func() {
			for _, s := range []step{{"GET", "/kv/" + keyDown, ``}, {"PUT", "/kv/" + keyDown, `{"x":2}`}, {"PATCH", "/kv/" + keyDown, `{"x":2}`}} {
				r := send(c.URL, s.method, s.path, s.body)
				Expect(r.status).To(Equal(503), s.method)
				Expect(r.body).To(MatchJSON(`{"error":"node_unavailable","message":"the owning node could not be reached; the request was not applied"}`))
			}
		})

		It("keeps serving keys on healthy nodes", Label("R-MN-FAIL-1"), func() {
			Expect(send(c.URL, "GET", "/kv/"+keyUp, "").status).To(Equal(200))
			Expect(send(c.URL, "PATCH", "/kv/"+keyUp, `{"x":2}`).body).To(HaveSuffix(`"version":2}` + "\n"))
		})

		It("still answers stateless validation errors itself", Label("R-MN-FAIL-5"), func() {
			Expect(send(c.URL, "PUT", "/kv/"+keyDown, `[1]`).status).To(Equal(400))
			Expect(send(c.URL, "PUT", "/kv/"+keyDown+"?ifVersion=0", `{}`).status).To(Equal(400))
			Expect(send(c.URL, "PUT", "/kv/"+keyDown, `{"a":"`+strings.Repeat("x", 1<<20)+`"}`).status).To(Equal(413))
			Expect(send(c.URL, "DELETE", "/kv/"+keyDown, ``).status).To(Equal(405))
		})

		It("serves its keys as never written once it restarts", Label("R-MN-FAIL-4"), func() {
			down.Restart()
			Expect(send(c.URL, "GET", "/kv/"+keyDown, "").status).To(Equal(404))
			Expect(send(c.URL, "GET", "/kv/"+keyUp, "").body).To(HaveSuffix(`"version":1}` + "\n"))
		})
	})
})
