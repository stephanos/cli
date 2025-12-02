package cliext

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/pflag"
)

// RootFlags contains the root-level flags that apply to all commands.
// These flags control logging, output format, and environment selection.
type RootFlags struct {
	// LogLevel controls the minimum log level (debug, info, warn, error, off).
	LogLevel string

	// LogFormat controls the log output format (text, json).
	LogFormat string

	// Output controls the output format (text, json, jsonl, none).
	Output string

	// Color controls color output (auto, always, never).
	Color string

	// Env specifies the environment/profile name from config.
	Env string
}

// AddFlags adds the root flags to a flag set.
// Typically called with cmd.PersistentFlags() on the root command.
//
// Example:
//
//	rootFlags := &cliext.RootFlags{}
//	rootFlags.AddFlags(rootCmd.PersistentFlags())
func (f *RootFlags) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&f.LogLevel, "log-level", "warn", "Log level: debug, info, warn, error, off")
	flags.StringVar(&f.LogFormat, "log-format", "text", "Log format: text, json")
	flags.StringVar(&f.Output, "output", "text", "Output format: text, json, jsonl, none")
	flags.StringVarP(&f.Color, "color", "", "auto", "Color output: auto, always, never")
	flags.StringVarP(&f.Env, "env", "e", "", "Environment name from config")
}

// NewLogger creates a *slog.Logger configured according to the root flags.
//
// Example:
//
//	logger, err := rootFlags.NewLogger()
//	if err != nil {
//	    return err
//	}
//	logger.Info("Starting", "version", "1.0.0")
func (f *RootFlags) NewLogger() (*slog.Logger, error) {
	level, err := f.parseSlogLevel()
	if err != nil {
		return nil, err
	}

	// If logging is disabled, return a no-op logger
	if level == slogLevelOff {
		return slog.New(newNoOpHandler()), nil
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	switch strings.ToLower(f.LogFormat) {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	case "text", "":
		handler = slog.NewTextHandler(os.Stderr, opts)
	default:
		return nil, fmt.Errorf("invalid log format: %s (expected: text, json)", f.LogFormat)
	}

	return slog.New(handler), nil
}

// parseSlogLevel converts the string log level to slog.Level.
func (f *RootFlags) parseSlogLevel() (slog.Level, error) {
	switch strings.ToLower(f.LogLevel) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning", "":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	case "off", "none", "disabled":
		return slogLevelOff, nil
	default:
		return slog.LevelWarn, fmt.Errorf("invalid log level: %s (expected: debug, info, warn, error, off)", f.LogLevel)
	}
}

// slogLevelOff is a level higher than error to effectively disable logging.
const slogLevelOff = slog.Level(100)

// noOpHandler is a slog handler that discards all logs.
type noOpHandler struct{}

func newNoOpHandler() *noOpHandler {
	return &noOpHandler{}
}

func (h *noOpHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return false
}

func (h *noOpHandler) Handle(_ context.Context, _ slog.Record) error {
	return nil
}

func (h *noOpHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *noOpHandler) WithGroup(_ string) slog.Handler {
	return h
}

// ClientFlags contains flags for connecting to a Temporal server.
type ClientFlags struct {
	// Address is the Temporal server address.
	Address string

	// Namespace is the Temporal namespace.
	Namespace string

	// TLS enables TLS for the connection.
	TLS bool

	// TLSCertPath is the path to the TLS certificate.
	TLSCertPath string

	// TLSKeyPath is the path to the TLS private key.
	TLSKeyPath string

	// TLSCACertPath is the path to the TLS CA certificate.
	TLSCACertPath string

	// TLSServerName is the server name for TLS verification.
	TLSServerName string

	// TLSDisableHostVerification disables TLS host verification.
	TLSDisableHostVerification bool

	// APIKey is the API key for authentication.
	APIKey string

	// CodecEndpoint is the codec server endpoint.
	CodecEndpoint string

	// CodecAuth is the codec server authorization header value.
	CodecAuth string
}

// AddFlags adds the client flags to a flag set.
// Typically called with cmd.PersistentFlags() on the root command.
//
// Example:
//
//	clientFlags := &cliext.ClientFlags{}
//	clientFlags.AddFlags(rootCmd.PersistentFlags())
func (f *ClientFlags) AddFlags(flags *pflag.FlagSet) {
	flags.StringVar(&f.Address, "address", "127.0.0.1:7233", "Temporal server address")
	flags.StringVarP(&f.Namespace, "namespace", "n", "default", "Temporal namespace")
	flags.BoolVar(&f.TLS, "tls", false, "Enable TLS")
	flags.StringVar(&f.TLSCertPath, "tls-cert-path", "", "Path to TLS certificate")
	flags.StringVar(&f.TLSKeyPath, "tls-key-path", "", "Path to TLS private key")
	flags.StringVar(&f.TLSCACertPath, "tls-ca-path", "", "Path to TLS CA certificate")
	flags.StringVar(&f.TLSServerName, "tls-server-name", "", "TLS server name override")
	flags.BoolVar(&f.TLSDisableHostVerification, "tls-disable-host-verification", false, "Disable TLS host verification")
	flags.StringVar(&f.APIKey, "api-key", "", "API key for authentication")
	flags.StringVar(&f.CodecEndpoint, "codec-endpoint", "", "Codec server endpoint")
	flags.StringVar(&f.CodecAuth, "codec-auth", "", "Codec server authorization header")
}

// TLSConfig creates a *tls.Config from the client flags.
// Returns nil if TLS is not enabled.
//
// Example:
//
//	tlsConfig, err := clientFlags.TLSConfig()
//	if err != nil {
//	    return err
//	}
func (f *ClientFlags) TLSConfig() (*tls.Config, error) {
	if !f.TLS && f.TLSCertPath == "" && f.TLSKeyPath == "" && f.TLSCACertPath == "" {
		return nil, nil
	}

	config := &tls.Config{
		ServerName:         f.TLSServerName,
		InsecureSkipVerify: f.TLSDisableHostVerification,
	}

	// Load client certificate if provided
	if f.TLSCertPath != "" && f.TLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(f.TLSCertPath, f.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	} else if f.TLSCertPath != "" || f.TLSKeyPath != "" {
		return nil, fmt.Errorf("both --tls-cert-path and --tls-key-path must be provided together")
	}

	// Load CA certificate if provided
	if f.TLSCACertPath != "" {
		caCert, err := os.ReadFile(f.TLSCACertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		config.RootCAs = caCertPool
	}

	return config, nil
}

// ApplyProfile applies values from a profile configuration to the client flags.
// Only empty/default values are overwritten; explicitly set flags take precedence.
func (f *ClientFlags) ApplyProfile(profile *Profile) {
	if profile == nil {
		return
	}

	// Only apply if not already set (default values)
	if f.Address == "127.0.0.1:7233" && profile.Address != "" {
		f.Address = profile.Address
	}
	if f.Namespace == "default" && profile.Namespace != "" {
		f.Namespace = profile.Namespace
	}
	if !f.TLS && profile.TLS {
		f.TLS = profile.TLS
	}
	if f.TLSCertPath == "" && profile.TLSCertPath != "" {
		f.TLSCertPath = profile.TLSCertPath
	}
	if f.TLSKeyPath == "" && profile.TLSKeyPath != "" {
		f.TLSKeyPath = profile.TLSKeyPath
	}
	if f.TLSCACertPath == "" && profile.TLSCACertPath != "" {
		f.TLSCACertPath = profile.TLSCACertPath
	}
	if f.TLSServerName == "" && profile.TLSServerName != "" {
		f.TLSServerName = profile.TLSServerName
	}
	if f.APIKey == "" && profile.APIKey != "" {
		f.APIKey = profile.APIKey
	}
	if f.CodecEndpoint == "" && profile.CodecEndpoint != "" {
		f.CodecEndpoint = profile.CodecEndpoint
	}
	if f.CodecAuth == "" && profile.CodecAuth != "" {
		f.CodecAuth = profile.CodecAuth
	}
}

// Profile represents connection settings that can be loaded from config.
// This is used by ApplyProfile to set default values.
type Profile struct {
	Address       string
	Namespace     string
	TLS           bool
	TLSCertPath   string
	TLSKeyPath    string
	TLSCACertPath string
	TLSServerName string
	APIKey        string
	CodecEndpoint string
	CodecAuth     string
}
