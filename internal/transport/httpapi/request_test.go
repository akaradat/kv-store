package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/core/kv"
	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

type parsed struct {
	handler string
	key     string
	write   httpapi.Write
}

type routed struct {
	served bool
	parsed parsed
	resp   *httptest.ResponseRecorder
}

// route sends one request through a chi router with the same {key} routes as node and router, whose
// handlers only parse.
func route(method, target, body string) routed {
	var out routed
	write := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			in, err := httpapi.ParseWrite(r)
			if err != nil {
				httpapi.WriteError(w, err)
				return
			}
			out.served, out.parsed = true, parsed{handler: name, write: in}
		}
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestSize(httpapi.MaxBodyBytes))
	r.Get("/kv/{key}", func(w http.ResponseWriter, r *http.Request) {
		key, err := httpapi.ParseKey(r)
		if err != nil {
			httpapi.WriteError(w, err)
			return
		}
		out.served, out.parsed = true, parsed{handler: "get", key: key}
	})
	r.Put("/kv/{key}", write("put"))
	r.Patch("/kv/{key}", write("patch"))
	out.resp = httptest.NewRecorder()
	r.ServeHTTP(out.resp, httptest.NewRequest(method, target, strings.NewReader(body)))
	return out
}

func errorCode(rec *httptest.ResponseRecorder) string {
	var body struct{ Error string }
	Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed(), rec.Body.String())
	return body.Error
}

func nested(levels int) string {
	return `{"a":` + strings.Repeat("[", levels-1) + strings.Repeat("]", levels-1) + `}`
}

var _ = Describe("ParseKey and ParseWrite", func() {
	DescribeTable("parses a valid request",
		func(method, target, body string, want parsed) {
			got := route(method, target, body)
			Expect(got.served).To(BeTrue(), "status %d: %s", got.resp.Code, got.resp.Body.String())
			Expect(got.parsed).To(Equal(want))
		},
		Entry("GET a key", Label("R-KEY-1"), "GET", "/kv/user:42", "", parsed{handler: "get", key: "user:42"}),
		Entry("PUT a key", Label("R-IFV-5"), "PUT", "/kv/a", `{"x":1}`, parsed{handler: "put", write: httpapi.Write{Key: "a", Body: []byte(`{"x":1}`)}}),
		Entry("PATCH with ifVersion", Label("R-IFV-1"), "PATCH", "/kv/a?ifVersion=7", `{}`, parsed{handler: "patch", write: httpapi.Write{Key: "a", IfVersion: 7, Body: []byte(`{}`)}}),
		Entry("GET ignores a malformed ifVersion", Label("R-IFV-7"), "GET", "/kv/a?ifVersion=abc", "", parsed{handler: "get", key: "a"}),
		Entry("PUT ignores unknown params", Label("R-IFV-8"), "PUT", "/kv/a?x=1&ifVersion=2&y", `{}`, parsed{handler: "put", write: httpapi.Write{Key: "a", IfVersion: 2, Body: []byte(`{}`)}}),
		Entry("the largest ifVersion", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=9223372036854775807", `{}`, parsed{handler: "put", write: httpapi.Write{Key: "a", IfVersion: 9223372036854775807, Body: []byte(`{}`)}}),
		Entry("trims whitespace around the body only", Label("R-BODY-1", "R-FID-1"), "PUT", "/kv/a", " \n\t{ \"x\" : 1 }\r\n ", parsed{handler: "put", write: httpapi.Write{Key: "a", Body: []byte(`{ "x" : 1 }`)}}),
		Entry("an empty object", Label("R-BODY-6"), "PUT", "/kv/a", `{}`, parsed{handler: "put", write: httpapi.Write{Key: "a", Body: []byte(`{}`)}}),
		Entry("duplicate field names", Label("R-BODY-7"), "PUT", "/kv/a", `{"n":1,"n":2}`, parsed{handler: "put", write: httpapi.Write{Key: "a", Body: []byte(`{"n":1,"n":2}`)}}),
		Entry("nesting of exactly 64 levels", Label("R-BODY-3"), "PUT", "/kv/a", nested(64), parsed{handler: "put", write: httpapi.Write{Key: "a", Body: []byte(nested(64))}}),
		Entry("brackets inside strings don't count as nesting", Label("R-BODY-3"), "PUT", "/kv/a", `{"s":"`+strings.Repeat("[", 100)+`\"{"}`, parsed{handler: "put", write: httpapi.Write{Key: "a", Body: []byte(`{"s":"` + strings.Repeat("[", 100) + `\"{"}`)}}),
	)

	DescribeTable("rejects with a JSON error",
		func(method, target, body string, status int, code string) {
			got := route(method, target, body)
			Expect(got.served).To(BeFalse())
			Expect(got.resp.Code).To(Equal(status))
			Expect(got.resp.Header().Get("Content-Type")).To(Equal("application/json"))
			Expect(errorCode(got.resp)).To(Equal(code))
		},
		Entry("a percent-encoded colon", Label("R-KEY-2"), "GET", "/kv/user%3A42", "", 400, "invalid_key"),
		Entry("a percent-encoded slash", Label("R-KEY-2"), "GET", "/kv/a%2Fb", "", 400, "invalid_key"),
		Entry("a percent-encoded letter", Label("R-KEY-2"), "GET", "/kv/%41", "", 400, "invalid_key"),
		Entry("an encoded space", Label("R-KEY-1"), "GET", "/kv/a%20b", "", 400, "invalid_key"),
		Entry("a non-ASCII key", Label("R-KEY-1"), "GET", "/kv/ключ", "", 400, "invalid_key"),
		Entry("a 257-character key", Label("R-KEY-1"), "GET", "/kv/"+strings.Repeat("k", 257), "", 400, "invalid_key"),
		Entry("the key .", Label("R-KEY-3"), "GET", "/kv/.", "", 400, "invalid_key"),
		Entry("the key ..", Label("R-KEY-3"), "GET", "/kv/..", "", 400, "invalid_key"),

		Entry("ifVersion empty", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion with no =", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion 0", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=0", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion negative", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=-1", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion leading zero", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=01", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion with +", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=+1", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion with encoded +", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=%2B1", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion decimal", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=1.0", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion exponent", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=1e2", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion with whitespace", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=%201", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion letters", Label("R-IFV-2"), "PATCH", "/kv/a?ifVersion=abc", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion overflow", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=9223372036854775808", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion far overflow", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=99999999999999999999", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion repeated with equal values", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=1&ifVersion=1", `{}`, 400, "invalid_if_version"),
		Entry("ifVersion repeated with different values", Label("R-IFV-2"), "PUT", "/kv/a?ifVersion=1&ifVersion=2", `{}`, 400, "invalid_if_version"),

		Entry("an array body", Label("R-BODY-2"), "PUT", "/kv/a", `[1,2]`, 400, "invalid_body"),
		Entry("a string body", Label("R-BODY-2"), "PUT", "/kv/a", `"x"`, 400, "invalid_body"),
		Entry("a number body", Label("R-BODY-2"), "PUT", "/kv/a", `42`, 400, "invalid_body"),
		Entry("a boolean body", Label("R-BODY-2"), "PUT", "/kv/a", `true`, 400, "invalid_body"),
		Entry("a null body", Label("R-BODY-2"), "PATCH", "/kv/a", `null`, 400, "invalid_body"),
		Entry("an empty body", Label("R-BODY-3"), "PUT", "/kv/a", ``, 400, "invalid_body"),
		Entry("a whitespace-only body", Label("R-BODY-3"), "PUT", "/kv/a", " \n ", 400, "invalid_body"),
		Entry("malformed JSON", Label("R-BODY-3"), "PUT", "/kv/a", `{bad`, 400, "invalid_body"),
		Entry("two objects", Label("R-BODY-3"), "PUT", "/kv/a", `{}{}`, 400, "invalid_body"),
		Entry("two objects with a space", Label("R-BODY-3"), "PUT", "/kv/a", `{} {}`, 400, "invalid_body"),
		Entry("trailing garbage", Label("R-BODY-3"), "PUT", "/kv/a", `{} x`, 400, "invalid_body"),
		Entry("invalid UTF-8 inside a string", Label("R-BODY-3"), "PUT", "/kv/a", "{\"a\":\"\xff\"}", 400, "invalid_body"),
		Entry("a byte-order mark", Label("R-BODY-3"), "PUT", "/kv/a", "\xef\xbb\xbf{}", 400, "invalid_body"),
		Entry("nesting of 65 levels", Label("R-BODY-3"), "PUT", "/kv/a", nested(65), 400, "invalid_body"),
		Entry("a body one byte over 1 MiB", Label("R-BODY-4"), "PUT", "/kv/a", `{"a":"`+strings.Repeat("x", httpapi.MaxBodyBytes-7)+`"}`, 413, "body_too_large"),
	)

	It("accepts a body of exactly 1 MiB", Label("R-BODY-4"), func() {
		body := `{"a":"` + strings.Repeat("x", httpapi.MaxBodyBytes-8) + `"}`
		Expect(body).To(HaveLen(httpapi.MaxBodyBytes))
		Expect(route("PUT", "/kv/a", body).served).To(BeTrue())
	})

	It("uses kv.Unconditional when ifVersion is absent", Label("R-IFV-5"), func() {
		Expect(route("PUT", "/kv/a", `{}`).parsed.write.IfVersion).To(Equal(kv.Unconditional))
	})

	DescribeTable("reports the first failure in R-ERR-4 order",
		func(method, target, body string, status int, code string) {
			got := route(method, target, body)
			Expect(got.resp.Code).To(Equal(status))
			if code != "" {
				Expect(errorCode(got.resp)).To(Equal(code))
			}
		},
		Entry("key before ifVersion", Label("R-ERR-4"), "PUT", "/kv/.?ifVersion=abc", `[1]`, 400, "invalid_key"),
		Entry("ifVersion before body", Label("R-ERR-4"), "PUT", "/kv/a?ifVersion=abc", `[1]`, 400, "invalid_if_version"),
		Entry("ifVersion before body size", Label("R-ERR-4"), "PUT", "/kv/a?ifVersion=0", strings.Repeat(" ", httpapi.MaxBodyBytes+1), 400, "invalid_if_version"),
		Entry("size before syntax", Label("R-ERR-4"), "PUT", "/kv/a", "["+strings.Repeat(" ", httpapi.MaxBodyBytes), 413, "body_too_large"),
	)
})
