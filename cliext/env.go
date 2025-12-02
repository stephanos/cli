package cliext

import "os"

// EnvLookup is an interface for environment variable lookups.
// This allows dependency injection for testing.
type EnvLookup interface {
	LookupEnv(key string) (string, bool)
	Environ() []string
}

// EnvLookupOS is the default EnvLookup implementation using os package.
var EnvLookupOS EnvLookup = envLookupOS{}

type envLookupOS struct{}

func (envLookupOS) LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

func (envLookupOS) Environ() []string {
	return os.Environ()
}

// MapEnvLookup is an EnvLookup implementation backed by a map.
// Useful for testing.
type MapEnvLookup struct {
	Env map[string]string
}

func (m MapEnvLookup) LookupEnv(key string) (string, bool) {
	val, ok := m.Env[key]
	return val, ok
}

func (m MapEnvLookup) Environ() []string {
	result := make([]string, 0, len(m.Env))
	for k, v := range m.Env {
		result = append(result, k+"="+v)
	}
	return result
}
