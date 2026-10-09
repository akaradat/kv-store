package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode/utf8"
)

const (
	MaxBodyBytes = 1 << 20
	maxDepth     = 64
)

// readBody expects the route to limit the body with middleware.RequestSize(MaxBodyBytes) (R-BODY-4).
func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return nil, bodyTooLarge()
	}
	if err != nil {
		return nil, invalidBody("could not read body: " + err.Error())
	}
	return validateBody(body)
}

func validateBody(body []byte) ([]byte, error) {
	// json.Valid accepts invalid UTF-8 inside strings.
	if !utf8.Valid(body) {
		return nil, invalidBody("body must be UTF-8")
	}
	body = bytes.Trim(body, " \t\r\n")
	switch {
	case len(body) == 0:
		return nil, invalidBody("body must not be empty")
	case !json.Valid(body):
		return nil, invalidBody("body must be one valid JSON value")
	case body[0] != '{':
		return nil, invalidBody("body must be a JSON object")
	case depth(body) > maxDepth:
		return nil, invalidBody("body must not nest deeper than 64 levels")
	}
	return body, nil
}

func depth(doc []byte) int {
	dec := json.NewDecoder(bytes.NewReader(doc))
	level, deepest := 0, 0
	for {
		token, err := dec.Token()
		if err != nil {
			return deepest
		}
		switch token {
		case json.Delim('{'), json.Delim('['):
			level++
			deepest = max(deepest, level)
		case json.Delim('}'), json.Delim(']'):
			level--
		}
	}
}
