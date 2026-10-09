package kv_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/kv"
)

var _ = Describe("ValidateKey", func() {
	DescribeTable("accepts",
		func(key string) {
			Expect(kv.ValidateKey(key)).To(Succeed())
		},
		Entry("letters, digits and colon", Label("R-KEY-1"), "user:42"),
		Entry("every allowed punctuation", Label("R-KEY-1"), "a:b_c.d-e"),
		Entry("one character", Label("R-KEY-1"), "x"),
		Entry("256 characters", Label("R-KEY-1"), strings.Repeat("k", 256)),
		Entry("three dots", Label("R-KEY-3"), "..."),
		Entry("leading dot", Label("R-KEY-3"), ".x"),
		Entry("inner dot", Label("R-KEY-3"), "a.b"),
		Entry("upper case", Label("R-KEY-4"), "User:1"),
	)

	DescribeTable("rejects",
		func(key string) {
			Expect(kv.ValidateKey(key)).NotTo(Succeed())
		},
		Entry("empty", Label("R-KEY-7"), ""),
		Entry("257 characters", Label("R-KEY-1"), strings.Repeat("k", 257)),
		Entry("space", Label("R-KEY-1"), "a b"),
		Entry("slash", Label("R-KEY-1"), "a/b"),
		Entry("non-ASCII", Label("R-KEY-1"), "ключ"),
		Entry("trailing newline", Label("R-KEY-1"), "a\n"),
		Entry("percent-encoded colon", Label("R-KEY-2"), "user%3A42"),
		Entry("single dot", Label("R-KEY-3"), "."),
		Entry("double dot", Label("R-KEY-3"), ".."),
	)
})
