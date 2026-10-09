package kv

import (
	"errors"
	"regexp"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9:_.-]{1,256}$`)

func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return errors.New("key must be 1 to 256 characters of A-Z a-z 0-9 : _ . -")
	}
	if key == "." || key == ".." {
		return errors.New(`key must not be "." or ".."`)
	}
	return nil
}
