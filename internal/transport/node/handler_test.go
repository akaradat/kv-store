package node_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/akaradat/kv-store/internal/servertest"
)

type response struct {
	status int
	header http.Header
	body   string
}

var _ = Describe("kvnode handler", func() {
	var url string

	BeforeEach(func() {
		url = servertest.StartNode("node-1").URL
	})

	send := func(method, path, body string, header ...string) response {
		req, err := http.NewRequest(method, url+path, strings.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i+1 < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		return response{status: resp.StatusCode, header: resp.Header, body: string(b)}
	}

	Describe("GET /kv/{key}", func() {
		It("returns 404 with an error body for a missing key", Label("R-ERR-1", "R-ERR-3"), func() {
			r := send("GET", "/kv/nope", "")
			Expect(r.status).To(Equal(404))
			Expect(r.header.Get("Content-Type")).To(Equal("application/json"))
			Expect(r.body).To(MatchJSON(`{"error":"not_found","message":"key not found"}`))
		})

		It("returns the record a PUT committed, without a node ID", Label("R-CON-5", "R-NODE-4", "R-FID-4"), func() {
			send("PUT", "/kv/user:42", `{"name":"Ari","points":10}`)
			r := send("GET", "/kv/user:42", "")
			Expect(r.status).To(Equal(200))
			Expect(r.body).To(Equal(`{"key":"user:42","value":{"name":"Ari","points":10},"version":1}` + "\n"))
		})

		It("treats keys as case-sensitive", Label("R-KEY-4"), func() {
			send("PUT", "/kv/user:1", `{}`)
			Expect(send("GET", "/kv/User:1", "").status).To(Equal(404))
		})

		It("returns number text and escapes exactly as sent", Label("R-FID-1"), func() {
			body := `{"n":12345678901234567890,"f":1.0,"s":"<A&>"}`
			send("PUT", "/kv/a", body)
			Expect(send("GET", "/kv/a", "").body).To(Equal(`{"key":"a","value":` + body + `,"version":1}` + "\n"))
		})
	})

	Describe("PUT /kv/{key}", func() {
		It("creates a key with 200 and version 1", Label("R-HTTP-3", "R-VER-1"), func() {
			r := send("PUT", "/kv/a", `{"x":1}`)
			Expect(r.status).To(Equal(200))
			Expect(r.body).To(Equal(`{"key":"a","value":{"x":1},"version":1}` + "\n"))
		})

		It("replaces the whole value and raises the version", Label("R-VER-2"), func() {
			send("PUT", "/kv/a", `{"name":"Ari","points":10}`)
			r := send("PUT", "/kv/a", `{"name":"Ari"}`)
			Expect(r.body).To(Equal(`{"key":"a","value":{"name":"Ari"},"version":2}` + "\n"))
		})

		It("writes when ifVersion matches and rejects a stale one with the current version", Label("R-IFV-3", "R-ERR-2"), func() {
			send("PUT", "/kv/a", `{"n":1}`)
			send("PUT", "/kv/a", `{"n":2}`)

			Expect(send("PUT", "/kv/a?ifVersion=2", `{"n":3}`).body).To(Equal(`{"key":"a","value":{"n":3},"version":3}` + "\n"))

			r := send("PUT", "/kv/a?ifVersion=2", `{"n":99}`)
			Expect(r.status).To(Equal(409))
			Expect(r.body).To(MatchJSON(`{"error":"version_conflict","message":"version conflict: current version is 3","currentVersion":3}`))
		})

		It("rejects ifVersion on a missing key with currentVersion null and creates nothing", Label("R-IFV-4", "R-ERR-2"), func() {
			r := send("PUT", "/kv/new-key?ifVersion=1", `{}`)
			Expect(r.status).To(Equal(409))
			Expect(r.body).To(ContainSubstring(`"currentVersion":null`))
			Expect(send("GET", "/kv/new-key", "").status).To(Equal(404))
			Expect(send("GET", "/kv", "").body).To(BeEmpty())
		})

		It("answers 400 rather than 409 for a stale guard with a bad body", Label("R-ERR-4"), func() {
			send("PUT", "/kv/a", `{}`)
			r := send("PUT", "/kv/a?ifVersion=9", `[1]`)
			Expect(r.status).To(Equal(400))
			Expect(r.body).To(ContainSubstring(`"invalid_body"`))
		})

		It("rejects a body over 1 MiB with 413 and stores nothing", Label("R-BODY-4"), func() {
			r := send("PUT", "/kv/a", `{"a":"`+strings.Repeat("x", 1<<20)+`"}`)
			Expect(r.status).To(Equal(413))
			Expect(r.body).To(MatchJSON(`{"error":"body_too_large","message":"body must be at most 1048576 bytes"}`))
			Expect(send("GET", "/kv/a", "").status).To(Equal(404))
		})

		It("ignores Content-Type", Label("R-BODY-5"), func() {
			r := send("PUT", "/kv/a", `{"x":1}`, "Content-Type", "text/plain")
			Expect(r.status).To(Equal(200))
		})

		It("ignores If-Match", Label("R-IFV-1"), func() {
			send("PUT", "/kv/a", `{}`)
			r := send("PUT", "/kv/a", `{}`, "If-Match", "99")
			Expect(r.status).To(Equal(200))
			Expect(r.body).To(HaveSuffix(`"version":2}` + "\n"))
		})
	})

	Describe("PATCH /kv/{key}", func() {
		It("creates a missing key from the delta", Label("R-VER-1"), func() {
			r := send("PATCH", "/kv/user:43", `{"name":"Bo"}`)
			Expect(r.body).To(Equal(`{"key":"user:43","value":{"name":"Bo"},"version":1}` + "\n"))
		})

		It("merges top-level fields and raises the version", Label("R-PATCH-1", "R-VER-2"), func() {
			send("PUT", "/kv/a", `{"name":"Ari","points":20}`)
			r := send("PATCH", "/kv/a", `{"rank":"gold","points":30}`)
			Expect(r.status).To(Equal(200))
			Expect(r.body).To(MatchJSON(`{"key":"a","value":{"name":"Ari","points":30,"rank":"gold"},"version":2}` + "\n"))
		})

		It("stores null instead of deleting the field", Label("R-PATCH-3"), func() {
			send("PUT", "/kv/a", `{"x":1}`)
			Expect(send("PATCH", "/kv/a", `{"x":null}`).body).To(MatchJSON(`{"key":"a","value":{"x":null},"version":2}` + "\n"))
		})

		It("rejects a stale ifVersion", Label("R-IFV-3"), func() {
			send("PUT", "/kv/a", `{}`)
			r := send("PATCH", "/kv/a?ifVersion=5", `{"rank":"silver"}`)
			Expect(r.status).To(Equal(409))
			Expect(r.body).To(ContainSubstring(`"currentVersion":1`))
		})
	})

	It("leaves value and version unchanged after failed requests", Label("R-VER-3"), func() {
		send("PUT", "/kv/a", `{"n":1}`)
		for _, bad := range []struct{ method, path, body string }{
			{"PUT", "/kv/a", `[1,2]`},
			{"PUT", "/kv/a", `null`},
			{"PATCH", "/kv/a", `{bad`},
			{"PUT", "/kv/a?ifVersion=0", `{}`},
			{"PUT", "/kv/a?ifVersion=7", `{"n":2}`},
			{"PATCH", "/kv/a?ifVersion=7", `{"n":2}`},
			{"DELETE", "/kv/a", ``},
		} {
			Expect(send(bad.method, bad.path, bad.body).status).To(BeNumerically(">=", 400), "%s %s", bad.method, bad.path)
		}
		Expect(send("GET", "/kv/a", "").body).To(Equal(`{"key":"a","value":{"n":1},"version":1}` + "\n"))
	})

	DescribeTable("answers routing errors",
		func(method, path string, status int, allow []string, body string) {
			r := send(method, path, "")
			Expect(r.status).To(Equal(status))
			Expect(r.header.Values("Allow")).To(ConsistOf(allow))
			if body == "" {
				Expect(r.body).To(BeEmpty())
				return
			}
			Expect(r.header.Get("Content-Type")).To(Equal("application/json"))
			Expect(r.body).To(MatchJSON(body))
		},
		Entry("DELETE on a key", Label("R-HTTP-1"), "DELETE", "/kv/a", 405, []string{"GET", "PUT", "PATCH"}, ""),
		Entry("POST on a key", Label("R-HTTP-1"), "POST", "/kv/a", 405, []string{"GET", "PUT", "PATCH"}, ""),
		Entry("HEAD on a key", Label("R-HTTP-1"), "HEAD", "/kv/a", 405, []string{"GET", "PUT", "PATCH"}, ""),
		Entry("POST on the list", Label("R-HTTP-1"), "POST", "/kv", 405, []string{"GET"}, ""),
		Entry("DELETE on the list", Label("R-HTTP-1"), "DELETE", "/kv", 405, []string{"GET"}, ""),
		Entry("method checked before key", Label("R-ERR-4"), "DELETE", "/kv/bad%20key", 405, []string{"GET", "PUT", "PATCH"}, ""),
		Entry("route checked before method", Label("R-ERR-4"), "DELETE", "/kv/a/b", 404, nil,
			`{"error":"not_found","message":"no such route"}`),
		Entry("a prefix that only looks like /kv", Label("R-HTTP-2"), "GET", "/kvx", 404, nil,
			`{"error":"not_found","message":"no such route"}`),
		Entry("a trailing slash", Label("R-KEY-6"), "GET", "/kv/a/", 404, nil,
			`{"error":"not_found","message":"no such route"}`),
		Entry("an unknown path", Label("R-HTTP-2"), "GET", "/", 404, nil,
			`{"error":"not_found","message":"no such route"}`),
		Entry("two segments", Label("R-KEY-6"), "GET", "/kv/a/b", 404, nil,
			`{"error":"not_found","message":"no such route"}`),
		Entry("an empty key", Label("R-KEY-7", "R-LIST-8"), "GET", "/kv/", 404, nil,
			`{"error":"not_found","message":"no such route"}`),
		Entry("an encoded colon", Label("R-KEY-2"), "GET", "/kv/user%3A42", 400, nil,
			`{"error":"invalid_key","message":"key must be 1 to 256 characters of A-Z a-z 0-9 : _ . -"}`),
	)

	It("answers a broken percent-escape with 400 before the handler runs", Label("R-KEY-2"), func() {
		conn, err := net.Dial("tcp", strings.TrimPrefix(url, "http://"))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(conn.Close)
		_, err = fmt.Fprint(conn, "GET /kv/%zz HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
		Expect(err).NotTo(HaveOccurred())

		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(400))
	})

	Describe("GET /kv", func() {
		It("returns 200 NDJSON with an empty body for an empty store", Label("R-LIST-1", "R-LIST-5"), func() {
			r := send("GET", "/kv", "")
			Expect(r.status).To(Equal(200))
			Expect(r.header.Get("Content-Type")).To(Equal("application/x-ndjson"))
			Expect(r.body).To(BeEmpty())
		})

		It("lists each key once with the node ID, every line ending in a newline", Label("R-LIST-2", "R-LIST-3", "R-LIST-4"), func() {
			for range 5 {
				send("PUT", "/kv/a", `{}`)
			}
			send("PATCH", "/kv/b", `{}`)

			r := send("GET", "/kv", "")
			Expect(r.body).To(HaveSuffix("\n"))
			Expect(strings.Split(strings.TrimSuffix(r.body, "\n"), "\n")).To(ConsistOf(
				`{"key":"a","node":"node-1"}`,
				`{"key":"b","node":"node-1"}`,
			))
		})

		It("ignores query parameters", Label("R-LIST-7"), func() {
			send("PUT", "/kv/a", `{}`)
			send("PUT", "/kv/b", `{}`)
			Expect(strings.Count(send("GET", "/kv?limit=1&prefix=x", "").body, "\n")).To(Equal(2))
		})
	})
})
