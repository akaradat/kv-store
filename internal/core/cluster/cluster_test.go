package cluster_test

import (
	"fmt"
	"math/rand/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/cluster"
)

func threeNodes() *cluster.Cluster {
	c, err := cluster.Parse([]byte(`{"nodes":[
		{"id":"node-1","url":"http://localhost:7001"},
		{"id":"node-2","url":"http://localhost:7002"},
		{"id":"node-3","url":"http://localhost:7003"}
	]}`))
	Expect(err).NotTo(HaveOccurred())
	return c
}

func owner(c *cluster.Cluster, key string) string {
	return c.Owner(cluster.Slot(key)).ID
}

var _ = Describe("Parse", func() {
	It("keeps nodes in file order", Label("R-MN-TOP-1"), func() {
		Expect(threeNodes().Nodes()).To(Equal([]cluster.Node{
			{ID: "node-1", URL: "http://localhost:7001"},
			{ID: "node-2", URL: "http://localhost:7002"},
			{ID: "node-3", URL: "http://localhost:7003"},
		}))
	})

	DescribeTable("rejects an invalid topology",
		func(config string) {
			_, err := cluster.Parse([]byte(config))
			Expect(err).To(HaveOccurred())
		},
		Entry("no nodes", Label("R-MN-TOP-2"), `{"nodes":[]}`),
		Entry("nodes missing", Label("R-MN-TOP-2"), `{}`),
		Entry("a duplicate ID", Label("R-MN-TOP-2"), `{"nodes":[{"id":"n","url":"http://a"},{"id":"n","url":"http://b"}]}`),
		Entry("an invalid ID", Label("R-MN-TOP-2"), `{"nodes":[{"id":"node 1","url":"http://a"}]}`),
		Entry("an empty ID", Label("R-MN-TOP-2"), `{"nodes":[{"id":"","url":"http://a"}]}`),
		Entry("a URL without scheme", Label("R-MN-CFG-1"), `{"nodes":[{"id":"n","url":"localhost:7001"}]}`),
		Entry("an empty URL", Label("R-MN-CFG-1"), `{"nodes":[{"id":"n","url":""}]}`),
		Entry("an unknown field", Label("R-MN-CFG-1"), `{"nodes":[{"id":"n","url":"http://a","weight":2}]}`),
		Entry("malformed JSON", Label("R-MN-CFG-1"), `{"nodes":`),
	)
})

var _ = Describe("slot ownership", func() {
	It("maps a key to the documented slot and owner", Label("R-MN-TOP-4"), func() {
		Expect(cluster.Slot("user:42")).To(Equal(390))
		Expect(owner(threeNodes(), "user:42")).To(Equal("node-2"))
	})

	It("splits 1024 slots into contiguous ranges of 342, 341 and 341", Label("R-MN-TOP-3"), func() {
		c := threeNodes()
		Expect(c.Owner(0).ID).To(Equal("node-1"))
		Expect(c.Owner(341).ID).To(Equal("node-1"))
		Expect(c.Owner(342).ID).To(Equal("node-2"))
		Expect(c.Owner(682).ID).To(Equal("node-2"))
		Expect(c.Owner(683).ID).To(Equal("node-3"))
		Expect(c.Owner(cluster.NumSlots - 1).ID).To(Equal("node-3"))
	})

	It("gives every slot to the only node when there is one", Label("R-MN-TOP-6"), func() {
		c, err := cluster.New(cluster.NewInput{Nodes: []cluster.Node{{ID: "solo", URL: "http://localhost:7000"}}})
		Expect(err).NotTo(HaveOccurred())
		for slot := range cluster.NumSlots {
			Expect(c.Owner(slot).ID).To(Equal("solo"))
		}
	})

	It("gives the same owner for the same key and topology every time", Label("R-MN-TOP-4"), func() {
		a, b := threeNodes(), threeNodes()
		for i := range 1000 {
			key := fmt.Sprintf("k-%d", i)
			Expect(owner(a, key)).To(Equal(owner(b, key)))
		}
	})

	It("spreads 30,000 random keys within 90–110% of the average per node", Label("R-MN-TOP-5"), func() {
		c := threeNodes()
		rng := rand.New(rand.NewPCG(1, 2))
		counts := map[string]int{}
		for range 30000 {
			counts[owner(c, fmt.Sprintf("k-%016x", rng.Uint64()))]++
		}
		Expect(counts).To(HaveLen(3))
		for id, n := range counts {
			Expect(n).To(BeNumerically(">=", 9000), id)
			Expect(n).To(BeNumerically("<=", 11000), id)
		}
	})
})
