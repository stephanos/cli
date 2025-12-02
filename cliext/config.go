package cliext

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
	"go.temporal.io/sdk/contrib/envconfig"
)

// LoadConfigOptions contains options for loading configuration.
type LoadConfigOptions struct {
	// ConfigFilePath is the path to the configuration file.
	// If empty, TEMPORAL_CONFIG_FILE env var is checked, then the default path is used.
	ConfigFilePath string

	// EnvLookup is used for environment variable lookups.
	// If nil, os.LookupEnv is used.
	EnvLookup EnvLookup
}

// LoadConfigResult contains the result of loading configuration.
type LoadConfigResult struct {
	// Config is the loaded configuration.
	Config *envconfig.ClientConfig

	// ConfigFilePath is the resolved path to the configuration file that was loaded.
	// This may differ from the input if TEMPORAL_CONFIG_FILE env var was used.
	ConfigFilePath string
}

// oauthConfigTOML is the TOML representation of OAuthConfig.
// We use a separate struct to control TOML field names and handle time.Time.
type oauthConfigTOML struct {
	ClientID      string            `toml:"client_id,omitempty"`
	ClientSecret  string            `toml:"client_secret,omitempty"`
	TokenURL      string            `toml:"token_url,omitempty"`
	AuthURL       string            `toml:"auth_url,omitempty"`
	AccessToken   string            `toml:"access_token,omitempty"`
	RefreshToken  string            `toml:"refresh_token,omitempty"`
	TokenType     string            `toml:"token_type,omitempty"`
	ExpiresAt     string            `toml:"expires_at,omitempty"` // RFC3339 format
	RequestParams map[string]string `toml:"request_params,omitempty"`
	Scopes        []string          `toml:"scopes,omitempty"`
}

// rawProfileWithOAuth is used to parse OAuth from a profile section.
type rawProfileWithOAuth struct {
	OAuth *oauthConfigTOML `toml:"oauth"`
}

// rawConfigWithOAuth is used to parse OAuth sections from the config file.
type rawConfigWithOAuth struct {
	Profiles map[string]*rawProfileWithOAuth `toml:"profiles"`
}

// LoadConfig loads the client configuration from the specified file or default location.
// If ConfigFilePath is empty, the TEMPORAL_CONFIG_FILE environment variable is checked.
//
// Example:
//
//	result, err := cliext.LoadConfig(cliext.LoadConfigOptions{})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Loaded config from %s\n", result.ConfigFilePath)
func LoadConfig(options LoadConfigOptions) (LoadConfigResult, error) {
	envLookup := options.EnvLookup
	if envLookup == nil {
		envLookup = EnvLookupOS
	}
	configFilePath := options.ConfigFilePath
	if configFilePath == "" {
		configFilePath, _ = envLookup.LookupEnv("TEMPORAL_CONFIG_FILE")
	}

	// Load base config using envconfig (ignores unknown sections like oauth)
	clientConfig, err := envconfig.LoadClientConfig(envconfig.LoadClientConfigOptions{
		ConfigFilePath: configFilePath,
		EnvLookup:      envLookup,
	})
	if err != nil {
		return LoadConfigResult{}, err
	}

	return LoadConfigResult{
		Config:         &clientConfig,
		ConfigFilePath: configFilePath,
	}, nil
}

// loadOAuthForProfile parses OAuth config for a specific profile from the config file.
func loadOAuthForProfile(configFilePath, profileName string) (*OAuthConfig, error) {
	if configFilePath == "" {
		var err error
		configFilePath, err = envconfig.DefaultConfigFilePath()
		if err != nil {
			return nil, nil // No default path, no OAuth config
		}
	}

	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var raw rawConfigWithOAuth
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	profile := raw.Profiles[profileName]
	if profile == nil || profile.OAuth == nil {
		return nil, nil
	}

	cfg := profile.OAuth
	oauth := &OAuthConfig{
		OAuthClientConfig: OAuthClientConfig{
			ClientID:      cfg.ClientID,
			ClientSecret:  cfg.ClientSecret,
			TokenURL:      cfg.TokenURL,
			AuthURL:       cfg.AuthURL,
			RequestParams: cfg.RequestParams,
			Scopes:        cfg.Scopes,
		},
		OAuthToken: OAuthToken{
			AccessToken:  cfg.AccessToken,
			RefreshToken: cfg.RefreshToken,
			TokenType:    cfg.TokenType,
		},
	}

	if cfg.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, cfg.ExpiresAt); err == nil {
			oauth.ExpiresAt = t
		}
	}

	return oauth, nil
}

// WriteConfig writes the configuration to the specified file or default location.
// The oauth map is keyed by profile name.
// If configFilePath is empty, the default path will be used.
//
// Example:
//
//	oauth := map[string]*cliext.OAuthConfig{
//	    "default": loginResult.Config,
//	}
//	err := cliext.WriteConfig(config, oauth, "")
func WriteConfig(config *envconfig.ClientConfig, oauth map[string]*OAuthConfig, configFilePath string) error {
	// Get file
	if configFilePath == "" {
		var err error
		if configFilePath, err = envconfig.DefaultConfigFilePath(); err != nil {
			return err
		}
	}

	// Convert base config to TOML
	b, err := config.ToTOML(envconfig.ClientConfigToTOMLOptions{})
	if err != nil {
		return fmt.Errorf("failed building TOML: %w", err)
	}

	// Append OAuth sections per profile
	if len(oauth) > 0 {
		var buf bytes.Buffer
		buf.Write(b)

		// Sort keys for deterministic output
		profileNames := make([]string, 0, len(oauth))
		for name := range oauth {
			profileNames = append(profileNames, name)
		}
		sort.Strings(profileNames)

		for _, profileName := range profileNames {
			cfg := oauth[profileName]
			if cfg == nil {
				continue
			}
			buf.WriteString(fmt.Sprintf("\n[profiles.%s.oauth]\n", profileName))
			writeOAuthTOML(&buf, cfg)
		}
		b = buf.Bytes()
	}

	// Write to file, making dirs as needed
	if err := os.MkdirAll(filepath.Dir(configFilePath), 0700); err != nil {
		return fmt.Errorf("failed making config file parent dirs: %w", err)
	}
	if err := os.WriteFile(configFilePath, b, 0600); err != nil {
		return fmt.Errorf("failed writing config file: %w", err)
	}
	return nil
}

// writeOAuthTOML writes OAuth config fields to a buffer in TOML format.
func writeOAuthTOML(buf *bytes.Buffer, cfg *OAuthConfig) {
	if cfg.ClientID != "" {
		buf.WriteString(fmt.Sprintf("client_id = %q\n", cfg.ClientID))
	}
	if cfg.ClientSecret != "" {
		buf.WriteString(fmt.Sprintf("client_secret = %q\n", cfg.ClientSecret))
	}
	if cfg.TokenURL != "" {
		buf.WriteString(fmt.Sprintf("token_url = %q\n", cfg.TokenURL))
	}
	if cfg.AuthURL != "" {
		buf.WriteString(fmt.Sprintf("auth_url = %q\n", cfg.AuthURL))
	}
	if cfg.AccessToken != "" {
		buf.WriteString(fmt.Sprintf("access_token = %q\n", cfg.AccessToken))
	}
	if cfg.RefreshToken != "" {
		buf.WriteString(fmt.Sprintf("refresh_token = %q\n", cfg.RefreshToken))
	}
	if cfg.TokenType != "" {
		buf.WriteString(fmt.Sprintf("token_type = %q\n", cfg.TokenType))
	}
	if !cfg.ExpiresAt.IsZero() {
		buf.WriteString(fmt.Sprintf("expires_at = %q\n", cfg.ExpiresAt.Format(time.RFC3339)))
	}
	if len(cfg.RequestParams) > 0 {
		// Sort keys for deterministic output
		keys := make([]string, 0, len(cfg.RequestParams))
		for k := range cfg.RequestParams {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// Use inline table syntax for proper TOML nesting
		buf.WriteString("request_params = { ")
		for i, k := range keys {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(fmt.Sprintf("%s = %q", k, cfg.RequestParams[k]))
		}
		buf.WriteString(" }\n")
	}
	if len(cfg.Scopes) > 0 {
		buf.WriteString("scopes = [")
		for i, scope := range cfg.Scopes {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(fmt.Sprintf("%q", scope))
		}
		buf.WriteString("]\n")
	}
}
