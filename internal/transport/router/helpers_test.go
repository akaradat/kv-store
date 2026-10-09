package router_test

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type response struct {
	status int
	body   string
}

func send(base, method, path, body string) response {
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	return response{status: resp.StatusCode, body: string(b)}
}

func readListUntilError(base string) (*http.Response, string, error) {
	resp, err := http.Get(base + "/kv")
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp, string(b), err
}

func startFake(h http.HandlerFunc) string {
	srv := httptest.NewServer(h)
	DeferCleanup(srv.Close)
	return srv.URL
}

func hijackAndClose(w http.ResponseWriter) {
	conn, _, err := http.NewResponseController(w).Hijack()
	if err == nil {
		conn.Close()
	}
}

func startNonHTTPServer() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(l.Close)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = http.ReadRequest(bufio.NewReader(conn))
				_, _ = conn.Write([]byte("this is not http\r\n\r\n"))
			}()
		}
	}()
	return "http://" + l.Addr().String()
}
