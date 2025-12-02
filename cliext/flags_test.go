package cliext

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootFlags_AddFlags(t *testing.T) {
	flags := &RootFlags{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.AddFlags(fs)

	// Check that all flags are registered
	assert.NotNil(t, fs.Lookup("log-level"))
	assert.NotNil(t, fs.Lookup("log-format"))
	assert.NotNil(t, fs.Lookup("output"))
	assert.NotNil(t, fs.Lookup("color"))
	assert.NotNil(t, fs.Lookup("env"))

	// Check default values
	assert.Equal(t, "warn", flags.LogLevel)
	assert.Equal(t, "text", flags.LogFormat)
	assert.Equal(t, "text", flags.Output)
	assert.Equal(t, "auto", flags.Color)
	assert.Equal(t, "", flags.Env)
}

func TestRootFlags_AddFlags_Parse(t *testing.T) {
	flags := &RootFlags{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.AddFlags(fs)

	err := fs.Parse([]string{
		"--log-level=debug",
		"--log-format=json",
		"--output=jsonl",
		"--color=always",
		"--env=production",
	})
	require.NoError(t, err)

	assert.Equal(t, "debug", flags.LogLevel)
	assert.Equal(t, "json", flags.LogFormat)
	assert.Equal(t, "jsonl", flags.Output)
	assert.Equal(t, "always", flags.Color)
	assert.Equal(t, "production", flags.Env)
}

func TestRootFlags_NewLogger(t *testing.T) {
	tests := []struct {
		name      string
		logLevel  string
		logFormat string
		expectErr bool
	}{
		{
			name:      "debug level text format",
			logLevel:  "debug",
			logFormat: "text",
		},
		{
			name:      "info level json format",
			logLevel:  "info",
			logFormat: "json",
		},
		{
			name:      "warn level default format",
			logLevel:  "warn",
			logFormat: "",
		},
		{
			name:      "error level",
			logLevel:  "error",
			logFormat: "text",
		},
		{
			name:      "off level",
			logLevel:  "off",
			logFormat: "text",
		},
		{
			name:      "invalid level",
			logLevel:  "invalid",
			logFormat: "text",
			expectErr: true,
		},
		{
			name:      "invalid format",
			logLevel:  "info",
			logFormat: "invalid",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := &RootFlags{
				LogLevel:  tt.logLevel,
				LogFormat: tt.logFormat,
			}

			logger, err := flags.NewLogger()

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, logger)

			// Test that logger works
			logger.Info("test message", "key", "value")
		})
	}
}

func TestRootFlags_NewLogger_Off(t *testing.T) {
	flags := &RootFlags{
		LogLevel:  "off",
		LogFormat: "text",
	}

	logger, err := flags.NewLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)

	// The no-op handler should not be enabled for any level
	handler := logger.Handler()
	assert.False(t, handler.Enabled(nil, slog.LevelDebug))
	assert.False(t, handler.Enabled(nil, slog.LevelInfo))
	assert.False(t, handler.Enabled(nil, slog.LevelWarn))
	assert.False(t, handler.Enabled(nil, slog.LevelError))
}

func TestClientFlags_AddFlags(t *testing.T) {
	flags := &ClientFlags{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.AddFlags(fs)

	// Check that all flags are registered
	assert.NotNil(t, fs.Lookup("address"))
	assert.NotNil(t, fs.Lookup("namespace"))
	assert.NotNil(t, fs.Lookup("tls"))
	assert.NotNil(t, fs.Lookup("tls-cert-path"))
	assert.NotNil(t, fs.Lookup("tls-key-path"))
	assert.NotNil(t, fs.Lookup("tls-ca-path"))
	assert.NotNil(t, fs.Lookup("tls-server-name"))
	assert.NotNil(t, fs.Lookup("tls-disable-host-verification"))
	assert.NotNil(t, fs.Lookup("api-key"))
	assert.NotNil(t, fs.Lookup("codec-endpoint"))
	assert.NotNil(t, fs.Lookup("codec-auth"))

	// Check default values
	assert.Equal(t, "127.0.0.1:7233", flags.Address)
	assert.Equal(t, "default", flags.Namespace)
	assert.False(t, flags.TLS)
}

func TestClientFlags_AddFlags_Parse(t *testing.T) {
	flags := &ClientFlags{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.AddFlags(fs)

	err := fs.Parse([]string{
		"--address=temporal.example.com:7233",
		"--namespace=production",
		"--tls",
		"--tls-cert-path=/path/to/cert.pem",
		"--tls-key-path=/path/to/key.pem",
		"--tls-ca-path=/path/to/ca.pem",
		"--tls-server-name=temporal.example.com",
		"--api-key=my-api-key",
	})
	require.NoError(t, err)

	assert.Equal(t, "temporal.example.com:7233", flags.Address)
	assert.Equal(t, "production", flags.Namespace)
	assert.True(t, flags.TLS)
	assert.Equal(t, "/path/to/cert.pem", flags.TLSCertPath)
	assert.Equal(t, "/path/to/key.pem", flags.TLSKeyPath)
	assert.Equal(t, "/path/to/ca.pem", flags.TLSCACertPath)
	assert.Equal(t, "temporal.example.com", flags.TLSServerName)
	assert.Equal(t, "my-api-key", flags.APIKey)
}

func TestClientFlags_TLSConfig(t *testing.T) {
	t.Run("no TLS", func(t *testing.T) {
		flags := &ClientFlags{}
		cfg, err := flags.TLSConfig()
		require.NoError(t, err)
		assert.Nil(t, cfg)
	})

	t.Run("TLS enabled only", func(t *testing.T) {
		flags := &ClientFlags{TLS: true}
		cfg, err := flags.TLSConfig()
		require.NoError(t, err)
		require.NotNil(t, cfg)
	})

	t.Run("with server name", func(t *testing.T) {
		flags := &ClientFlags{
			TLS:           true,
			TLSServerName: "temporal.example.com",
		}
		cfg, err := flags.TLSConfig()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "temporal.example.com", cfg.ServerName)
	})

	t.Run("disable host verification", func(t *testing.T) {
		flags := &ClientFlags{
			TLS:                        true,
			TLSDisableHostVerification: true,
		}
		cfg, err := flags.TLSConfig()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.True(t, cfg.InsecureSkipVerify)
	})

	t.Run("cert without key", func(t *testing.T) {
		flags := &ClientFlags{
			TLSCertPath: "/path/to/cert.pem",
		}
		_, err := flags.TLSConfig()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both --tls-cert-path and --tls-key-path")
	})

	t.Run("key without cert", func(t *testing.T) {
		flags := &ClientFlags{
			TLSKeyPath: "/path/to/key.pem",
		}
		_, err := flags.TLSConfig()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both --tls-cert-path and --tls-key-path")
	})

	t.Run("invalid cert path", func(t *testing.T) {
		flags := &ClientFlags{
			TLSCertPath: "/nonexistent/cert.pem",
			TLSKeyPath:  "/nonexistent/key.pem",
		}
		_, err := flags.TLSConfig()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load client certificate")
	})

	t.Run("invalid CA path", func(t *testing.T) {
		flags := &ClientFlags{
			TLS:           true,
			TLSCACertPath: "/nonexistent/ca.pem",
		}
		_, err := flags.TLSConfig()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read CA certificate")
	})
}

func TestClientFlags_ApplyProfile(t *testing.T) {
	t.Run("apply to defaults", func(t *testing.T) {
		flags := &ClientFlags{
			Address:   "127.0.0.1:7233",
			Namespace: "default",
		}

		profile := &Profile{
			Address:   "temporal.example.com:7233",
			Namespace: "production",
			TLS:       true,
			APIKey:    "profile-api-key",
		}

		flags.ApplyProfile(profile)

		assert.Equal(t, "temporal.example.com:7233", flags.Address)
		assert.Equal(t, "production", flags.Namespace)
		assert.True(t, flags.TLS)
		assert.Equal(t, "profile-api-key", flags.APIKey)
	})

	t.Run("explicit values take precedence", func(t *testing.T) {
		flags := &ClientFlags{
			Address:   "custom.example.com:7233",
			Namespace: "custom",
			APIKey:    "explicit-api-key",
		}

		profile := &Profile{
			Address:   "temporal.example.com:7233",
			Namespace: "production",
			APIKey:    "profile-api-key",
		}

		flags.ApplyProfile(profile)

		// These should NOT change because they differ from defaults
		assert.Equal(t, "custom.example.com:7233", flags.Address)
		assert.Equal(t, "custom", flags.Namespace)
		assert.Equal(t, "explicit-api-key", flags.APIKey)
	})

	t.Run("nil profile", func(t *testing.T) {
		flags := &ClientFlags{
			Address:   "127.0.0.1:7233",
			Namespace: "default",
		}

		flags.ApplyProfile(nil)

		// Should remain unchanged
		assert.Equal(t, "127.0.0.1:7233", flags.Address)
		assert.Equal(t, "default", flags.Namespace)
	})
}

func TestNoOpHandler(t *testing.T) {
	handler := newNoOpHandler()

	// Should never be enabled
	assert.False(t, handler.Enabled(nil, slog.LevelDebug))
	assert.False(t, handler.Enabled(nil, slog.LevelError))

	// Handle should succeed but do nothing
	err := handler.Handle(nil, slog.Record{})
	assert.NoError(t, err)

	// WithAttrs should return same handler
	assert.Equal(t, handler, handler.WithAttrs([]slog.Attr{}))

	// WithGroup should return same handler
	assert.Equal(t, handler, handler.WithGroup("test"))
}

func TestRootFlags_LogFormat(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer

	t.Run("text format", func(t *testing.T) {
		flags := &RootFlags{
			LogLevel:  "info",
			LogFormat: "text",
		}

		logger, err := flags.NewLogger()
		require.NoError(t, err)
		require.NotNil(t, logger)
	})

	t.Run("json format", func(t *testing.T) {
		flags := &RootFlags{
			LogLevel:  "info",
			LogFormat: "json",
		}

		logger, err := flags.NewLogger()
		require.NoError(t, err)
		require.NotNil(t, logger)
	})

	_ = buf // unused but shows we could capture output
}
