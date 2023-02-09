package constants

import "errors"

const PersistentStateTarget = "/usr/local/.state"

func DefaultRWPaths() []string {
	// Default RW_PATHS to mount if there are none defined
	return []string{"/etc", "/root", "/home", "/opt", "/srv", "/usr/local", "/var"}
}

var ErrAlreadyMounted = errors.New("already mounted")
