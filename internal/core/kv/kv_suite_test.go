package kv_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKV(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "kv")
}
