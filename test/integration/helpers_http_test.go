package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	. "github.com/onsi/gomega"
)

type response struct {
	status      int
	contentType string
	allow       string
	body        string
}

func send(base, method, path, body string) response {
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	return response{
		status:      resp.StatusCode,
		contentType: resp.Header.Get("Content-Type"),
		allow:       allowSet(resp.Header),
		body:        string(b),
	}
}

// allowSet ignores order: chi lists the allowed methods in a different order on each request.
func allowSet(h http.Header) string {
	methods := h.Values("Allow")
	slices.Sort(methods)
	return strings.Join(methods, ",")
}

func lines(body string) []string {
	if body == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(body, "\n"), "\n")
}

type listLine struct {
	Key  string `json:"key"`
	Node string `json:"node"`
}

func parseLines(body string) []listLine {
	var out []listLine
	for _, l := range lines(body) {
		var ll listLine
		Expect(json.Unmarshal([]byte(l), &ll)).To(Succeed(), l)
		out = append(out, ll)
	}
	return out
}
