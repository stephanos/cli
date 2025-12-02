package cliext

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/contrib/envconfig"
)

func TestLoadOAuthForProfile(t *testing.T) {
	tests := []struct {
		name        string
		configTOML  string
		profileName string
		expected    *OAuthConfig
		expectErr   bool
	}{
		{
			name: "full oauth config with request params",
			configTOML: `
[profiles.default.oauth]
client_id = "my-client-id"
client_secret = "my-secret"
token_url = "https://example.com/oauth/token"
auth_url = "https://example.com/oauth/authorize"
access_token = "access123"
refresh_token = "refresh456"
token_type = "Bearer"
expires_at = "2024-12-31T23:59:59Z"
scopes = ["openid", "profile"]

[profiles.default.oauth.request_params]
audience = "https://api.example.com"
custom_param = "custom_value"
`,
			profileName: "default",
			expected: &OAuthConfig{
				OAuthClientConfig: OAuthClientConfig{
					ClientID:     "my-client-id",
					ClientSecret: "my-secret",
					TokenURL:     "https://example.com/oauth/token",
					AuthURL:      "https://example.com/oauth/authorize",
					RequestParams: map[string]string{
						"audience":     "https://api.example.com",
						"custom_param": "custom_value",
					},
					Scopes: []string{"openid", "profile"},
				},
				OAuthToken: OAuthToken{
					AccessToken:  "access123",
					RefreshToken: "refresh456",
					TokenType:    "Bearer",
					ExpiresAt:    time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC),
				},
			},
		},
		{
			name: "minimal oauth config",
			configTOML: `
[profiles.prod.oauth]
client_id = "prod-client"
token_url = "https://prod.example.com/token"
auth_url = "https://prod.example.com/auth"
`,
			profileName: "prod",
			expected: &OAuthConfig{
				OAuthClientConfig: OAuthClientConfig{
					ClientID: "prod-client",
					TokenURL: "https://prod.example.com/token",
					AuthURL:  "https://prod.example.com/auth",
				},
			},
		},
		{
			name: "profile without oauth",
			configTOML: `
[profiles.default]
address = "localhost:7233"
`,
			profileName: "default",
			expected:    nil,
		},
		{
			name:        "nonexistent profile",
			configTOML:  `[profiles.other.oauth]`,
			profileName: "default",
			expected:    nil,
		},
		{
			name:        "empty config",
			configTOML:  "",
			profileName: "default",
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")

			err := os.WriteFile(configPath, []byte(tt.configTOML), 0600)
			require.NoError(t, err)

			result, err := loadOAuthForProfile(configPath, tt.profileName)

			if tt.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expected.ClientID, result.ClientID)
			assert.Equal(t, tt.expected.ClientSecret, result.ClientSecret)
			assert.Equal(t, tt.expected.TokenURL, result.TokenURL)
			assert.Equal(t, tt.expected.AuthURL, result.AuthURL)
			assert.Equal(t, tt.expected.RequestParams, result.RequestParams)
			assert.Equal(t, tt.expected.Scopes, result.Scopes)
			assert.Equal(t, tt.expected.AccessToken, result.AccessToken)
			assert.Equal(t, tt.expected.RefreshToken, result.RefreshToken)
			assert.Equal(t, tt.expected.TokenType, result.TokenType)
			assert.Equal(t, tt.expected.ExpiresAt, result.ExpiresAt)
		})
	}
}

func TestLoadOAuthForProfile_FileNotExists(t *testing.T) {
	result, err := loadOAuthForProfile("/nonexistent/path/config.toml", "default")
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestWriteConfig_WithOAuth(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	config := &envconfig.ClientConfig{}

	oauth := map[string]*OAuthConfig{
		"default": {
			OAuthClientConfig: OAuthClientConfig{
				ClientID:     "test-client",
				ClientSecret: "test-secret",
				TokenURL:     "https://example.com/token",
				AuthURL:      "https://example.com/auth",
				RequestParams: map[string]string{
					"audience": "https://api.example.com",
					"foo":      "bar",
				},
				Scopes: []string{"openid", "offline_access"},
			},
			OAuthToken: OAuthToken{
				AccessToken:  "access-token",
				RefreshToken: "refresh-token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	err := WriteConfig(config, oauth, configPath)
	require.NoError(t, err)

	// Read back and verify
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	content := string(data)

	// Verify OAuth section exists
	assert.Contains(t, content, "[profiles.default.oauth]")
	assert.Contains(t, content, `client_id = "test-client"`)
	assert.Contains(t, content, `client_secret = "test-secret"`)
	assert.Contains(t, content, `token_url = "https://example.com/token"`)
	assert.Contains(t, content, `auth_url = "https://example.com/auth"`)
	assert.Contains(t, content, `access_token = "access-token"`)
	assert.Contains(t, content, `refresh_token = "refresh-token"`)
	assert.Contains(t, content, `token_type = "Bearer"`)
	assert.Contains(t, content, `expires_at = "2024-06-15T12:00:00Z"`)
	assert.Contains(t, content, `scopes = ["openid", "offline_access"]`)

	// Verify request_params inline table
	assert.Contains(t, content, `request_params = {`)
	assert.Contains(t, content, `audience = "https://api.example.com"`)
	assert.Contains(t, content, `foo = "bar"`)

	// Verify we can read it back
	loaded, err := loadOAuthForProfile(configPath, "default")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, oauth["default"].ClientID, loaded.ClientID)
	assert.Equal(t, oauth["default"].RequestParams, loaded.RequestParams)
	assert.Equal(t, oauth["default"].Scopes, loaded.Scopes)
}

func TestWriteConfig_MultipleProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	config := &envconfig.ClientConfig{}

	oauth := map[string]*OAuthConfig{
		"dev": {
			OAuthClientConfig: OAuthClientConfig{
				ClientID: "dev-client",
				TokenURL: "https://dev.example.com/token",
				AuthURL:  "https://dev.example.com/auth",
			},
		},
		"prod": {
			OAuthClientConfig: OAuthClientConfig{
				ClientID: "prod-client",
				TokenURL: "https://prod.example.com/token",
				AuthURL:  "https://prod.example.com/auth",
				RequestParams: map[string]string{
					"audience": "https://prod-api.example.com",
				},
			},
		},
	}

	err := WriteConfig(config, oauth, configPath)
	require.NoError(t, err)

	// Verify both profiles
	devOAuth, err := loadOAuthForProfile(configPath, "dev")
	require.NoError(t, err)
	require.NotNil(t, devOAuth)
	assert.Equal(t, "dev-client", devOAuth.ClientID)

	prodOAuth, err := loadOAuthForProfile(configPath, "prod")
	require.NoError(t, err)
	require.NotNil(t, prodOAuth)
	assert.Equal(t, "prod-client", prodOAuth.ClientID)
	assert.Equal(t, map[string]string{"audience": "https://prod-api.example.com"}, prodOAuth.RequestParams)
}

func TestWriteConfig_NoOAuth(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	config := &envconfig.ClientConfig{}

	err := WriteConfig(config, nil, configPath)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Should not contain OAuth sections
	assert.NotContains(t, string(data), "[oauth]")
	assert.NotContains(t, string(data), "client_id")
}

func TestWriteConfig_EmptyRequestParams(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	config := &envconfig.ClientConfig{}

	oauth := map[string]*OAuthConfig{
		"default": {
			OAuthClientConfig: OAuthClientConfig{
				ClientID: "test-client",
				TokenURL: "https://example.com/token",
				AuthURL:  "https://example.com/auth",
				// No RequestParams
			},
		},
	}

	err := WriteConfig(config, oauth, configPath)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Should not contain request_params section
	assert.NotContains(t, string(data), "[request_params]")
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	configTOML := `
[profiles.default]
address = "localhost:7233"
namespace = "my-namespace"
`
	err := os.WriteFile(configPath, []byte(configTOML), 0600)
	require.NoError(t, err)

	result, err := LoadConfig(LoadConfigOptions{
		ConfigFilePath: configPath,
	})
	require.NoError(t, err)

	assert.NotNil(t, result.Config)
}

func TestLoadConfig_WithEnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	configTOML := `
[profiles.default]
address = "localhost:7233"
`
	err := os.WriteFile(configPath, []byte(configTOML), 0600)
	require.NoError(t, err)

	// Use custom env lookup
	envLookup := MapEnvLookup{
		Env: map[string]string{
			"TEMPORAL_CONFIG_FILE": configPath,
		},
	}

	result, err := LoadConfig(LoadConfigOptions{
		EnvLookup: envLookup,
	})
	require.NoError(t, err)

	assert.NotNil(t, result.Config)
	assert.Equal(t, configPath, result.ConfigFilePath)
}
