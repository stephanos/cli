package cliext

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockOAuthServer(t *testing.T) {
	t.Run("server starts and provides endpoints", func(t *testing.T) {
		server := NewMockOAuthServer()
		defer server.Close()

		assert.NotEmpty(t, server.DeviceAuthURL())
		assert.NotEmpty(t, server.TokenURL())
		assert.Contains(t, server.DeviceAuthURL(), "/oauth/device/code")
		assert.Contains(t, server.TokenURL(), "/oauth/token")
	})

	t.Run("device code endpoint returns configured response", func(t *testing.T) {
		server := NewMockOAuthServer()
		defer server.Close()

		data := url.Values{}
		data.Set("client_id", "test-client")
		data.Set("scope", "openid profile")

		resp, err := http.PostForm(server.DeviceAuthURL(), data)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "test-client", server.LastDeviceCodeRequest.Get("client_id"))
	})

	t.Run("device code endpoint returns error when configured", func(t *testing.T) {
		server := NewMockOAuthServer()
		server.DeviceCodeError = "invalid_client"
		server.DeviceCodeErrorDesc = "Unknown client"
		defer server.Close()

		data := url.Values{}
		data.Set("client_id", "bad-client")

		resp, err := http.PostForm(server.DeviceAuthURL(), data)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("token endpoint returns configured tokens", func(t *testing.T) {
		server := NewMockOAuthServer()
		defer server.Close()

		data := url.Values{}
		data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		data.Set("device_code", "mock-device-code")
		data.Set("client_id", "test-client")

		resp, err := http.PostForm(server.TokenURL(), data)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("token endpoint returns pending when configured", func(t *testing.T) {
		server := NewMockOAuthServer()
		server.Pending = true
		defer server.Close()

		data := url.Values{}
		data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		data.Set("device_code", "mock-device-code")

		resp, err := http.PostForm(server.TokenURL(), data)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("tracks last token request", func(t *testing.T) {
		server := NewMockOAuthServer()
		defer server.Close()

		data := url.Values{}
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", "my-refresh-token")
		data.Set("client_id", "test-client")

		_, err := http.PostForm(server.TokenURL(), data)
		require.NoError(t, err)

		assert.Equal(t, "refresh_token", server.LastTokenRequest.Get("grant_type"))
		assert.Equal(t, "my-refresh-token", server.LastTokenRequest.Get("refresh_token"))
	})
}

func TestOAuthClient_Login_DeviceCodeFlow(t *testing.T) {
	t.Run("successful device code login", func(t *testing.T) {
		server := NewMockOAuthServer()
		defer server.Close()

		var capturedURL, capturedCode string

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			AuthURL:  server.DeviceAuthURL(),
			TokenURL: server.TokenURL(),
			Scopes:   []string{"openid", "profile"},
		})
		client.OnVerification = func(url, code string) {
			capturedURL = url
			capturedCode = code
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		token, err := client.Login(ctx)

		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Equal(t, "mock-access-token", token.AccessToken)
		assert.Equal(t, "mock-refresh-token", token.RefreshToken)
		assert.Equal(t, "Bearer", token.TokenType)
		assert.NotEmpty(t, capturedURL)
		assert.Equal(t, "MOCK-CODE", capturedCode)
	})

	t.Run("login with device code error", func(t *testing.T) {
		server := NewMockOAuthServer()
		server.DeviceCodeError = "invalid_client"
		server.DeviceCodeErrorDesc = "Unknown client"
		defer server.Close()

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "bad-client",
			AuthURL:  server.DeviceAuthURL(),
			TokenURL: server.TokenURL(),
		})
		client.OnVerification = func(url, code string) {}

		ctx := context.Background()
		_, err := client.Login(ctx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to request device code")
	})

	t.Run("login with context cancellation", func(t *testing.T) {
		server := NewMockOAuthServer()
		server.Pending = true // Keep returning pending
		defer server.Close()

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			AuthURL:  server.DeviceAuthURL(),
			TokenURL: server.TokenURL(),
		})
		client.OnVerification = func(url, code string) {}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := client.Login(ctx)

		assert.Error(t, err)
	})
}

func TestOAuthClient_Token(t *testing.T) {
	server := NewMockOAuthServer()
	defer server.Close()

	t.Run("returns existing token when not expired", func(t *testing.T) {
		config := &OAuthConfig{
			OAuthToken: OAuthToken{
				AccessToken:  "existing-access-token",
				RefreshToken: "existing-refresh-token",
				ExpiresAt:    time.Now().Add(10 * time.Minute), // Valid for 10 more minutes
			},
		}

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: server.TokenURL(),
		})

		ctx := context.Background()
		token, err := client.Token(ctx, config)

		require.NoError(t, err)
		assert.Equal(t, "existing-access-token", token.AccessToken)
		assert.Equal(t, "existing-refresh-token", token.RefreshToken)
		// Should not have made a token request
		assert.Empty(t, server.LastTokenRequest)
	})

	t.Run("refreshes token when expired", func(t *testing.T) {
		config := &OAuthConfig{
			OAuthToken: OAuthToken{
				AccessToken:  "old-access-token",
				RefreshToken: "old-refresh-token",
				ExpiresAt:    time.Now().Add(-1 * time.Minute), // Expired 1 minute ago
			},
		}

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: server.TokenURL(),
		})

		ctx := context.Background()
		token, err := client.Token(ctx, config)

		require.NoError(t, err)
		assert.Equal(t, "mock-access-token", token.AccessToken)
		assert.Equal(t, "mock-refresh-token", token.RefreshToken)
		// Config should be updated in place
		assert.Equal(t, "mock-access-token", config.AccessToken)
		assert.Equal(t, "mock-refresh-token", config.RefreshToken)
	})

	t.Run("refreshes token within grace period", func(t *testing.T) {
		// Create a new server for this test to check if refresh was called
		testServer := NewMockOAuthServer()
		defer testServer.Close()

		config := &OAuthConfig{
			OAuthToken: OAuthToken{
				AccessToken:  "old-access-token",
				RefreshToken: "old-refresh-token",
				ExpiresAt:    time.Now().Add(30 * time.Second), // Expires in 30 seconds (within 1-minute grace)
			},
		}

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: testServer.TokenURL(),
		})

		ctx := context.Background()
		token, err := client.Token(ctx, config)

		require.NoError(t, err)
		// Should have refreshed the token
		assert.Equal(t, "mock-access-token", token.AccessToken)
		assert.NotEmpty(t, testServer.LastTokenRequest, "should have attempted token refresh")
	})

	t.Run("successful token fetch", func(t *testing.T) {
		config := &OAuthConfig{
			OAuthToken: OAuthToken{RefreshToken: "old-refresh-token"},
		}

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: server.TokenURL(),
		})

		ctx := context.Background()
		token, err := client.Token(ctx, config)

		require.NoError(t, err)
		assert.Equal(t, "mock-access-token", token.AccessToken)
		assert.Equal(t, "mock-refresh-token", token.RefreshToken)
		// Config should be updated in place
		assert.Equal(t, "mock-access-token", config.AccessToken)
		assert.Equal(t, "mock-refresh-token", config.RefreshToken)

		assert.Equal(t, "refresh_token", server.LastTokenRequest.Get("grant_type"))
		assert.Equal(t, "old-refresh-token", server.LastTokenRequest.Get("refresh_token"))
	})

	t.Run("token with nil config", func(t *testing.T) {
		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: server.TokenURL(),
		})

		ctx := context.Background()
		_, err := client.Token(ctx, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no refresh token available")
	})

	t.Run("token with empty config", func(t *testing.T) {
		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: server.TokenURL(),
		})

		ctx := context.Background()
		_, err := client.Token(ctx, &OAuthConfig{})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no refresh token available")
	})

	t.Run("token returns server error", func(t *testing.T) {
		server := NewMockOAuthServer()
		server.TokenError = "invalid_grant"
		server.TokenErrorDescription = "Refresh token expired"
		defer server.Close()

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: server.TokenURL(),
		})

		ctx := context.Background()
		_, err := client.Token(ctx, &OAuthConfig{OAuthToken: OAuthToken{RefreshToken: "expired-token"}})

		assert.Error(t, err)
	})

	t.Run("invalid_grant triggers OnRefreshError callback", func(t *testing.T) {
		server := NewMockOAuthServer()
		server.TokenError = "invalid_grant"
		server.TokenErrorDescription = "Refresh token expired"
		defer server.Close()

		reloginCalled := false
		reloginToken := &OAuthToken{
			AccessToken:  "relogin-access-token",
			RefreshToken: "relogin-refresh-token",
			TokenType:    "Bearer",
		}

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: server.TokenURL(),
		})
		client.OnRefreshError = func(ctx context.Context, refreshErr error) (*OAuthToken, error) {
			reloginCalled = true
			return reloginToken, nil
		}

		config := &OAuthConfig{OAuthToken: OAuthToken{RefreshToken: "expired-token"}}
		ctx := context.Background()
		token, err := client.Token(ctx, config)

		require.NoError(t, err)
		assert.True(t, reloginCalled)
		assert.Equal(t, "relogin-access-token", token.AccessToken)
		assert.Equal(t, "relogin-refresh-token", token.RefreshToken)
		// Config should be updated
		assert.Equal(t, "relogin-access-token", config.AccessToken)
	})

	t.Run("OnRefreshError can propagate error", func(t *testing.T) {
		server := NewMockOAuthServer()
		server.TokenError = "invalid_grant"
		server.TokenErrorDescription = "Refresh token expired"
		defer server.Close()

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: server.TokenURL(),
		})
		// Explicitly set OnRefreshError to nil to disable handling
		client.OnRefreshError = nil

		ctx := context.Background()
		_, err := client.Token(ctx, &OAuthConfig{OAuthToken: OAuthToken{RefreshToken: "expired-token"}})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to refresh token")
	})

	t.Run("custom IsTokenExpired function", func(t *testing.T) {
		testServer := NewMockOAuthServer()
		defer testServer.Close()

		config := &OAuthConfig{
			OAuthToken: OAuthToken{
				AccessToken:  "old-access-token",
				RefreshToken: "old-refresh-token",
				ExpiresAt:    time.Now().Add(10 * time.Minute), // Would be valid with default
			},
		}

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: testServer.TokenURL(),
		})
		// Custom: always refresh
		client.IsTokenExpired = func(expiresAt time.Time) bool {
			return true
		}

		ctx := context.Background()
		token, err := client.Token(ctx, config)

		require.NoError(t, err)
		// Should have refreshed despite token being valid by default standards
		assert.Equal(t, "mock-access-token", token.AccessToken)
		assert.NotEmpty(t, testServer.LastTokenRequest, "should have attempted token refresh")
	})

	t.Run("IsTokenExpired can prevent refresh", func(t *testing.T) {
		testServer := NewMockOAuthServer()
		defer testServer.Close()

		config := &OAuthConfig{
			OAuthToken: OAuthToken{
				AccessToken:  "existing-access-token",
				RefreshToken: "existing-refresh-token",
				ExpiresAt:    time.Now().Add(-10 * time.Minute), // Expired by default standards
			},
		}

		client := NewOAuthClient(OAuthClientConfig{
			ClientID: "test-client",
			TokenURL: testServer.TokenURL(),
		})
		// Custom: never refresh
		client.IsTokenExpired = func(expiresAt time.Time) bool {
			return false
		}

		ctx := context.Background()
		token, err := client.Token(ctx, config)

		require.NoError(t, err)
		// Should NOT have refreshed
		assert.Equal(t, "existing-access-token", token.AccessToken)
		assert.Empty(t, testServer.LastTokenRequest, "should NOT have attempted token refresh")
	})
}
