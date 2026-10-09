package httpapi

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/akaradat/kv-store/internal/core/kv"
)

var positiveInteger = regexp.MustCompile(`^[1-9][0-9]*$`)

// parseIfVersion reads the raw query by hand: url.ParseQuery silently drops a malformed pair, which
// would turn a guarded write into an unconditional one (S1).
func parseIfVersion(rawQuery string) (uint64, error) {
	var raw string
	found := false
	for pair := range strings.SplitSeq(rawQuery, "&") {
		name, value, _ := strings.Cut(pair, "=")
		if name != "ifVersion" {
			continue
		}
		if found {
			return 0, invalidIfVersion("ifVersion must be given at most once")
		}
		raw, found = value, true
	}
	if !found {
		return kv.Unconditional, nil
	}

	if !positiveInteger.MatchString(raw) {
		return 0, invalidIfVersion("ifVersion must be a positive integer without sign or leading zeros")
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, invalidIfVersion("ifVersion must be at most 9223372036854775807")
	}
	return uint64(n), nil
}
