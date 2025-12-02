package cliext

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvLookupOS(t *testing.T) {
	t.Run("LookupEnv existing", func(t *testing.T) {
		// PATH should always exist
		val, ok := EnvLookupOS.LookupEnv("PATH")
		assert.True(t, ok)
		assert.NotEmpty(t, val)
	})

	t.Run("LookupEnv non-existing", func(t *testing.T) {
		val, ok := EnvLookupOS.LookupEnv("CLIEXT_TEST_NONEXISTENT_VAR_12345")
		assert.False(t, ok)
		assert.Empty(t, val)
	})

	t.Run("Environ returns environment", func(t *testing.T) {
		env := EnvLookupOS.Environ()
		assert.NotEmpty(t, env)
		// Should contain PATH
		found := false
		for _, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				found = true
				break
			}
		}
		assert.True(t, found, "Environ should contain PATH")
	})
}

func TestMapEnvLookup(t *testing.T) {
	t.Run("LookupEnv existing", func(t *testing.T) {
		lookup := MapEnvLookup{
			Env: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		}

		val, ok := lookup.LookupEnv("FOO")
		assert.True(t, ok)
		assert.Equal(t, "bar", val)
	})

	t.Run("LookupEnv non-existing", func(t *testing.T) {
		lookup := MapEnvLookup{
			Env: map[string]string{
				"FOO": "bar",
			},
		}

		val, ok := lookup.LookupEnv("MISSING")
		assert.False(t, ok)
		assert.Empty(t, val)
	})

	t.Run("LookupEnv empty value", func(t *testing.T) {
		lookup := MapEnvLookup{
			Env: map[string]string{
				"EMPTY": "",
			},
		}

		val, ok := lookup.LookupEnv("EMPTY")
		assert.True(t, ok)
		assert.Empty(t, val)
	})

	t.Run("Environ returns all vars", func(t *testing.T) {
		lookup := MapEnvLookup{
			Env: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		}

		env := lookup.Environ()
		sort.Strings(env)
		assert.Equal(t, []string{"BAZ=qux", "FOO=bar"}, env)
	})

	t.Run("Environ with nil map", func(t *testing.T) {
		lookup := MapEnvLookup{}
		env := lookup.Environ()
		assert.Empty(t, env)
	})
}

func TestMapEnvLookup_Integration(t *testing.T) {
	// Test that MapEnvLookup satisfies the EnvLookup interface
	var lookup EnvLookup = MapEnvLookup{
		Env: map[string]string{
			"TEMPORAL_PROFILE":     "test",
			"TEMPORAL_CONFIG_FILE": "/path/to/config",
		},
	}

	val, ok := lookup.LookupEnv("TEMPORAL_PROFILE")
	assert.True(t, ok)
	assert.Equal(t, "test", val)

	env := lookup.Environ()
	assert.Len(t, env, 2)
}

func TestEnvLookupOS_SetEnv(t *testing.T) {
	// Test with a custom env var we set
	key := "CLIEXT_TEST_VAR_" + t.Name()
	expected := "test_value_123"

	// Set the env var
	err := os.Setenv(key, expected)
	assert.NoError(t, err)
	defer os.Unsetenv(key)

	// Verify via EnvLookupOS
	val, ok := EnvLookupOS.LookupEnv(key)
	assert.True(t, ok)
	assert.Equal(t, expected, val)
}
