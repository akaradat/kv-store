//go:build e2e

package e2e_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

const exitTimeout = 10 * time.Second

var _ = Describe("the kvnode binary", func() {
	It("serves the API on -addr and reports -node-id only in GET /kv", Label("R-NODE-1", "R-NODE-4"), func() {
		n := startNode("node-7")
		Expect(send(n.URL, "PUT", "/kv/a", `{"x":1}`).body).To(Equal(`{"key":"a","value":{"x":1},"version":1}` + "\n"))
		Expect(send(n.URL, "GET", "/kv", "").body).To(Equal(`{"key":"a","node":"node-7"}` + "\n"))
	})

	It("defaults the node ID to node-1", Label("R-NODE-3"), func() {
		addr := freeAddr()
		start(bin.node, nil, "-addr", addr)
		waitUntilServing("http://" + addr)
		send("http://"+addr, "PUT", "/kv/a", `{}`)
		Expect(send("http://"+addr, "GET", "/kv", "").body).To(Equal(`{"key":"a","node":"node-1"}` + "\n"))
	})

	It("reads the node ID from KV_NODE_ID, and the flag wins over it", Label("R-NODE-1"), func() {
		fromEnv, fromFlag := freeAddr(), freeAddr()
		start(bin.node, []string{"KV_NODE_ID=env-node"}, "-addr", fromEnv)
		start(bin.node, []string{"KV_NODE_ID=env-node"}, "-addr", fromFlag, "-node-id", "flag-node")
		for addr, want := range map[string]string{fromEnv: "env-node", fromFlag: "flag-node"} {
			waitUntilServing("http://" + addr)
			send("http://"+addr, "PUT", "/kv/a", `{}`)
			Expect(send("http://"+addr, "GET", "/kv", "").body).To(ContainSubstring(`"node":"` + want + `"`))
		}
	})

	It("refuses to start with an invalid node ID", Label("R-NODE-2"), func() {
		s := start(bin.node, nil, "-addr", freeAddr(), "-node-id", "node 1")
		Eventually(s).WithTimeout(exitTimeout).Should(gexec.Exit(1))
		Expect(s.Err).To(gbytes.Say("invalid -node-id"))
	})

	It("exits cleanly on SIGTERM", Label("S7"), func() {
		n := startNode("node-1")
		Eventually(n.session.Terminate()).WithTimeout(exitTimeout).Should(gexec.Exit(0))
	})

	It("passes the counter test run by the kvcounter binary", Label("R-CON-1", "R-CON-4"), func() {
		n := startNode("node-1")
		s := start(bin.counter, nil, "-url", n.URL)
		Eventually(s).WithTimeout(time.Minute).Should(gexec.Exit(0))
		Expect(s.Out).To(gbytes.Say("final: count=300 version=301  PASS"))
	})
})

var _ = Describe("the kvrouter binary in front of three kvnode processes", func() {
	var d *deployment

	BeforeEach(func() {
		d = startDeployment(3)
	})

	It("serves the single-node API", Label("R-MN-API-1"), func() {
		Expect(send(d.URL, "PUT", "/kv/user:42", `{"name":"Ari"}`).body).To(HaveSuffix(`"version":1}` + "\n"))
		Expect(send(d.URL, "PUT", "/kv/user:42?ifVersion=5", `{}`).status).To(Equal(409))
		Expect(send(d.URL, "PATCH", "/kv/user:42?ifVersion=1", `{"points":3}`).body).To(Equal(`{"key":"user:42","value":{"name":"Ari","points":3},"version":2}` + "\n"))
		Expect(send(d.URL, "GET", "/kv/user:42", "").status).To(Equal(200))
	})

	It("lists every key once with the ID of the node that owns it", Label("R-MN-LIST-1", "R-MN-CFG-2"), func() {
		for _, n := range d.Nodes {
			Expect(send(d.URL, "PUT", "/kv/"+d.keyOn(n), `{}`).status).To(Equal(200))
		}
		var listed []listLine
		for _, n := range d.Nodes {
			listed = append(listed, listLine{Key: d.keyOn(n), Node: n.ID})
		}
		Expect(parseLines(send(d.URL, "GET", "/kv", "").body)).To(ConsistOf(listed))
	})

	It("answers 503 for a crashed node's keys, and 404 once it restarts empty", Label("R-MN-FAIL-1", "R-MN-FAIL-4"), func() {
		down, up := d.Nodes[1], d.Nodes[0]
		send(d.URL, "PUT", "/kv/"+d.keyOn(down), `{}`)
		send(d.URL, "PUT", "/kv/"+d.keyOn(up), `{}`)

		down.Kill()
		Expect(send(d.URL, "GET", "/kv/"+d.keyOn(down), "").status).To(Equal(503))
		Expect(send(d.URL, "GET", "/kv/"+d.keyOn(up), "").status).To(Equal(200))
		Expect(send(d.URL, "GET", "/kv", "").status).To(Equal(503))

		down.Restart()
		Expect(send(d.URL, "GET", "/kv/"+d.keyOn(down), "").status).To(Equal(404))
		Expect(send(d.URL, "GET", "/kv", "").status).To(Equal(200))
	})

	It("passes the counter test run by the kvcounter binary", Label("R-MN-CON-1"), func() {
		s := start(bin.counter, nil, "-url", d.URL)
		Eventually(s).WithTimeout(time.Minute).Should(gexec.Exit(0))
		Expect(s.Out).To(gbytes.Say("final: count=300 version=301  PASS"))
	})

	It("exits cleanly on SIGTERM", Label("S7"), func() {
		Eventually(d.router.Terminate()).WithTimeout(exitTimeout).Should(gexec.Exit(0))
	})
})

var _ = Describe("kvrouter configuration", func() {
	It("refuses to start without -cluster", Label("R-MN-CFG-1"), func() {
		s := start(bin.router, nil, "-addr", freeAddr())
		Eventually(s).WithTimeout(exitTimeout).Should(gexec.Exit(1))
		Expect(s.Err).To(gbytes.Say("-cluster is required"))
	})

	It("refuses a topology with a duplicate node ID", Label("R-MN-CFG-1"), func() {
		config := writeFile("cluster.json", []byte(`{"nodes":[{"id":"node-1","url":"http://127.0.0.1:1"},{"id":"node-1","url":"http://127.0.0.1:2"}]}`))
		s := start(bin.router, nil, "-addr", freeAddr(), "-cluster", config)
		Eventually(s).WithTimeout(exitTimeout).Should(gexec.Exit(1))
		Expect(s.Err).To(gbytes.Say("duplicate node ID"))
	})
})
