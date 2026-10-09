package cluster

import (
	"errors"
	"regexp"
)

var nodeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func ValidateNodeID(id string) error {
	if !nodeIDPattern.MatchString(id) {
		return errors.New("node ID must be 1 to 64 characters of A-Z a-z 0-9 _ -")
	}
	return nil
}
