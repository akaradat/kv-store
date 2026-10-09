package httpapi_test

import (
	"errors"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/kv"
	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

var _ = Describe("WriteError", func() {
	DescribeTable("maps errors to status and body",
		func(err error, status int, body string) {
			w := httptest.NewRecorder()
			httpapi.WriteError(w, err)
			Expect(w.Code).To(Equal(status))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
			Expect(w.Body.String()).To(MatchJSON(body))
		},
		Entry("missing key", Label("R-ERR-1", "R-ERR-3"), kv.ErrNotFound, 404,
			`{"error":"not_found","message":"key not found"}`),
		Entry("conflict on an existing key", Label("R-ERR-2"), &kv.VersionConflictError{Current: 7}, 409,
			`{"error":"version_conflict","message":"version conflict: current version is 7","currentVersion":7}`),
		Entry("conflict on a missing key", Label("R-ERR-2"), &kv.VersionConflictError{}, 409,
			`{"error":"version_conflict","message":"version conflict: key does not exist","currentVersion":null}`),
		Entry("anything unexpected", Label("R-ERR-1"), errors.New("boom"), 500,
			`{"error":"internal_error","message":"boom"}`),
	)
})
