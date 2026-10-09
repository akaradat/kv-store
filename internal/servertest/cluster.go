package servertest

import (
	"fmt"
	"log/slog"
	"net/http/httptest"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/cluster"
	"github.com/akaradat/kv-store/internal/transport/router"
)

const NodeTimeout = 2 * time.Second

type Cluster struct {
	URL      string
	Nodes    []*Node
	Topology *cluster.Cluster
}

// StartCluster starts n nodes named node-1 … node-n behind a router.
func StartCluster(n int) *Cluster {
	c := &Cluster{}
	topology := make([]cluster.Node, n)
	for i := range n {
		node := StartNode(fmt.Sprintf("node-%d", i+1))
		c.Nodes = append(c.Nodes, node)
		topology[i] = cluster.Node{ID: node.ID, URL: node.URL}
	}
	c.Topology, c.URL = StartRouter(topology, NodeTimeout)
	return c
}

func StartRouter(nodes []cluster.Node, nodeTimeout time.Duration) (*cluster.Cluster, string) {
	topology, err := cluster.New(cluster.NewInput{Nodes: nodes})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	srv := httptest.NewServer(router.NewHandler(router.NewHandlerInput{Cluster: topology, NodeTimeout: nodeTimeout, Log: slog.New(slog.DiscardHandler)}))
	ginkgo.DeferCleanup(srv.Close)
	return topology, srv.URL
}

func (c *Cluster) Owner(key string) *Node {
	id := c.Topology.Owner(cluster.Slot(key)).ID
	for _, n := range c.Nodes {
		if n.ID == id {
			return n
		}
	}
	ginkgo.Fail("servertest: topology names unknown node " + id)
	return nil
}

// KeyOn returns the first key of the form prefix-0, prefix-1, … that the node owns.
func (c *Cluster) KeyOn(n *Node, prefix string) string {
	for i := 0; ; i++ {
		key := fmt.Sprintf("%s-%d", prefix, i)
		if c.Owner(key) == n {
			return key
		}
	}
}
