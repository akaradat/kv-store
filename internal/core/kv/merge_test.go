package kv_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/kv"
)

var _ = Describe("Merge", func() {
	DescribeTable("shallow-merges the delta over the stored object",
		func(stored, delta, want string) {
			got, err := kv.Merge([]byte(stored), []byte(delta))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(got)).To(MatchJSON(want))
		},
		Entry("adds a new field and keeps the others", Label("R-PATCH-1"),
			`{"name":"Ari","points":10}`, `{"rank":"gold"}`, `{"name":"Ari","points":10,"rank":"gold"}`),
		Entry("overwrites an existing field", Label("R-PATCH-1"),
			`{"name":"Ari","points":10}`, `{"points":30}`, `{"name":"Ari","points":30}`),
		Entry("replaces a nested object instead of merging it", Label("R-PATCH-2"),
			`{"o":{"a":1}}`, `{"o":{"b":1}}`, `{"o":{"b":1}}`),
		Entry("stores null literally", Label("R-PATCH-3"),
			`{"x":1,"y":2}`, `{"x":null}`, `{"x":null,"y":2}`),
		Entry("leaves the value unchanged for an empty delta", Label("R-PATCH-4"),
			`{"name":"Ari"}`, `{}`, `{"name":"Ari"}`),
		Entry("merges into an empty object", Label("R-PATCH-1"),
			`{}`, `{"a":1}`, `{"a":1}`),
	)

	It("keeps nested number text and does not escape HTML characters", Label("R-FID-2"), func() {
		got, err := kv.Merge(
			[]byte(`{"big":12345678901234567891,"f":1.0,"o":{"n":98765432109876543210}}`),
			[]byte(`{"s":"<a&b>"}`),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(got)).To(Equal(`{"big":12345678901234567891,"f":1.0,"o":{"n":98765432109876543210},"s":"<a&b>"}`))
	})

	It("still produces a valid object when names are duplicated", Label("R-FID-3"), func() {
		got, err := kv.Merge([]byte(`{"a":1,"a":2}`), []byte(`{"b":1,"b":2}`))
		Expect(err).NotTo(HaveOccurred())
		var obj map[string]any
		Expect(json.Unmarshal(got, &obj)).To(Succeed())
		Expect(obj).To(HaveKey("a"))
		Expect(obj).To(HaveKey("b"))
	})
})
