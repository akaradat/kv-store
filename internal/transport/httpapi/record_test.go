package httpapi_test

import (
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/kv"
	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

var _ = Describe("WriteRecord", func() {
	It("keeps the stored text except whitespace outside strings, without escaping <, > or &", Label("R-FID-1", "R-FID-4"), func() {
		w := httptest.NewRecorder()
		value := `{ "s" : "<a&b> c", "n" : 12345678901234567890, "f": 1.0, "e":"\u0041", "d":1, "d":2 }`

		httpapi.WriteRecord(w, kv.Entry{Key: "user:42", Value: []byte(value), Version: 7})

		Expect(w.Code).To(Equal(200))
		Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
		Expect(w.Body.String()).To(Equal(`{"key":"user:42","value":{"s":"<a&b> c","n":12345678901234567890,"f":1.0,"e":"\u0041","d":1,"d":2},"version":7}` + "\n"))
	})
})
