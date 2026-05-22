// Package config loads service configuration from the environment.
//
// The platform deliberately uses environment variables only — Vault and the
// External Secrets Operator project secrets into the pod's env; ConfigMaps
// project non-secret config the same way. This keeps every service runnable
// with `task run:<service>` without a config-file scavenger hunt.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// String returns the value of the named env var, or def if unset/empty.
func String(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// Required returns the value of the named env var, or an error if it's missing.
// Use this for values that cannot have a meaningful default (database DSN,
// downstream addresses, signing keys).
func Required(name string) (string, error) {
	v := os.Getenv(name)
	if v == "" {
		return "", fmt.Errorf("required env var %q is not set", name)
	}
	return v, nil
}

func Int(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func Bool(name string, def bool) bool {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func Duration(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
