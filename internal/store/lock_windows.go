//go:build windows

package store

import "errors"

func lockFile(string) (func(), error) {
	return nil, errors.New("local installation locking is not supported on Windows in this phase")
}
