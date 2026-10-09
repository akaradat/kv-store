package kv

import (
	"errors"
	"strconv"
)

var ErrNotFound = errors.New("key not found")

type VersionConflictError struct {
	Current uint64 // 0 when the key doesn't exist, written as null (R-ERR-2)
}

func (e *VersionConflictError) Error() string {
	if e.Current == 0 {
		return "version conflict: key does not exist"
	}
	return "version conflict: current version is " + strconv.FormatUint(e.Current, 10)
}
