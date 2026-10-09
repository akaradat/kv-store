package servertest

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/store"
	"github.com/akaradat/kv-store/internal/transport/node"
)

type Node struct {
	ID  string
	URL string
	srv *httptest.Server
}

// StartNode must be called from a spec or setup node: the server is closed by DeferCleanup.
func StartNode(id string) *Node {
	n := &Node{ID: id}
	n.serve(httptest.NewServer(newNodeHandler(id)))
	return n
}

func (n *Node) Stop() {
	n.srv.Close()
}

// Restart brings the node back empty on the same address, as a restarted in-memory process would.
func (n *Node) Restart() {
	l, err := net.Listen("tcp", n.srv.Listener.Addr().String())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	srv := httptest.NewUnstartedServer(newNodeHandler(n.ID))
	srv.Listener.Close()
	srv.Listener = l
	srv.Start()
	n.serve(srv)
}

func (n *Node) serve(srv *httptest.Server) {
	ginkgo.DeferCleanup(srv.Close)
	n.srv = srv
	n.URL = srv.URL
}

func newNodeHandler(id string) http.Handler {
	return node.NewHandler(node.NewHandlerInput{Store: store.New(), NodeID: id, Log: slog.New(slog.DiscardHandler)})
}
