//go:build e2e

package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

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

type listLine struct {
	Key  string `json:"key"`
	Node string `json:"node"`
}

func parseLines(body string) []listLine {
	var out []listLine
	for line := range strings.Lines(body) {
		var ll listLine
		Expect(json.Unmarshal([]byte(line), &ll)).To(Succeed(), line)
		out = append(out, ll)
	}
	return out
}
