//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/akaradat/kv-store/internal/core/cluster"
)

const startupTimeout = 10 * time.Second

var bin struct{ node, router, counter string }

var _ = BeforeSuite(func() {
	build := func(pkg string) string {
		path, err := gexec.Build("github.com/akaradat/kv-store/"+pkg, "-race")
		Expect(err).NotTo(HaveOccurred())
		return path
	}
	bin.node, bin.router, bin.counter = build("cmd/kvnode"), build("cmd/kvrouter"), build("test/e2e/kvcounter")
	DeferCleanup(gexec.CleanupBuildArtifacts)
})

// start drops the caller's KV_* variables so only the spec decides a process's configuration.
func start(path string, env []string, args ...string) *gexec.Session {
	cmd := exec.Command(path, args...)
	cmd.Env = append(slices.DeleteFunc(os.Environ(), func(v string) bool { return strings.HasPrefix(v, "KV_") }), env...)
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { session.Kill().Wait() })
	return session
}

func freeAddr() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	defer l.Close()
	return l.Addr().String()
}

func waitUntilServing(url string) {
	Eventually(func() error {
		resp, err := http.Get(url + "/kv")
		if err == nil {
			resp.Body.Close()
		}
		return err
	}).WithTimeout(startupTimeout).Should(Succeed())
}

type nodeProcess struct {
	ID      string
	URL     string
	addr    string
	session *gexec.Session
}

func startNode(id string) *nodeProcess {
	n := &nodeProcess{ID: id, addr: freeAddr()}
	n.URL = "http://" + n.addr
	n.Restart()
	return n
}

func (n *nodeProcess) Kill() {
	n.session.Kill().Wait()
}

// Restart starts the node on the same address, as an operator would after a crash.
func (n *nodeProcess) Restart() {
	n.session = start(bin.node, nil, "-addr", n.addr, "-node-id", n.ID)
	waitUntilServing(n.URL)
}

type deployment struct {
	URL      string
	Nodes    []*nodeProcess
	Topology *cluster.Cluster
	router   *gexec.Session
}

func startDeployment(nodeCount int) *deployment {
	d := &deployment{}
	var topology []cluster.Node
	for i := range nodeCount {
		n := startNode(fmt.Sprintf("node-%d", i+1))
		d.Nodes = append(d.Nodes, n)
		topology = append(topology, cluster.Node{ID: n.ID, URL: n.URL})
	}
	config, err := json.Marshal(map[string]any{"nodes": topology})
	Expect(err).NotTo(HaveOccurred())
	d.Topology, err = cluster.Parse(config)
	Expect(err).NotTo(HaveOccurred())

	addr := freeAddr()
	d.URL = "http://" + addr
	d.router = start(bin.router, nil, "-addr", addr, "-cluster", writeFile("cluster.json", config), "-node-timeout", "1s")
	waitUntilServing(d.URL)
	return d
}

func (d *deployment) owner(key string) *nodeProcess {
	id := d.Topology.Owner(cluster.Slot(key)).ID
	return d.Nodes[slices.IndexFunc(d.Nodes, func(n *nodeProcess) bool { return n.ID == id })]
}

func (d *deployment) keyOn(n *nodeProcess) string {
	for i := 0; ; i++ {
		key := fmt.Sprintf("key-%d", i)
		if d.owner(key) == n {
			return key
		}
	}
}

func writeFile(name string, data []byte) string {
	path := filepath.Join(GinkgoT().TempDir(), name)
	Expect(os.WriteFile(path, data, 0o600)).To(Succeed())
	return path
}
