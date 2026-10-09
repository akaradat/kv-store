package cluster_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/cluster"
)

var _ = Describe("ValidateNodeID", func() {
	DescribeTable("accepts",
		func(id string) {
			Expect(cluster.ValidateNodeID(id)).To(Succeed())
		},
		Entry("the default", Label("R-NODE-2"), "node-1"),
		Entry("underscore and digits", Label("R-NODE-2"), "Node_42"),
		Entry("64 characters", Label("R-NODE-2"), strings.Repeat("n", 64)),
	)

	DescribeTable("rejects",
		func(id string) {
			Expect(cluster.ValidateNodeID(id)).NotTo(Succeed())
		},
		Entry("empty", Label("R-NODE-2"), ""),
		Entry("65 characters", Label("R-NODE-2"), strings.Repeat("n", 65)),
		Entry("colon", Label("R-NODE-2"), "node:1"),
		Entry("dot", Label("R-NODE-2"), "node.1"),
		Entry("space", Label("R-NODE-2"), "node 1"),
		Entry("trailing newline", Label("R-NODE-2"), "node-1\n"),
	)
})
