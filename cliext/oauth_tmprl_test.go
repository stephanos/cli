package cliext

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuthClient_TemporalCloud tests the OAuth client against Temporal Cloud.
// This is an interactive test that requires manual user authentication.
func TestOAuthClient_TemporalCloud(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping interactive test in CI")
	}

	client := NewOAuthClient(OAuthClientConfig{
		ClientID: "d7V5bZMLCbRLfRVpqC567AqjAERaWHhl",
		AuthURL:  "https://login.tmprl.cloud/oauth/device/code",
		TokenURL: "https://login.tmprl.cloud/oauth/token",
		RequestParams: map[string]string{
			"audience": "https://saas-api.tmprl.cloud",
		},
		Scopes: []string{"openid", "profile", "user", "offline_access"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	token, err := client.Login(ctx)
	require.NoError(t, err, "Login failed")

	assert.NotEmpty(t, token.AccessToken, "AccessToken should not be empty")
	assert.NotEmpty(t, token.RefreshToken, "RefreshToken should not be empty")
	assert.False(t, token.ExpiresAt.IsZero(), "ExpiresAt should not be zero")

	t.Logf("Login successful, token expires at %v", token.ExpiresAt)

	// Test token refresh using the Token() method
	config := &OAuthConfig{
		OAuthClientConfig: client.Options,
		OAuthToken:        *token,
	}

	// Force expiration check by setting IsTokenExpired to always return true
	client.IsTokenExpired = func(expiresAt time.Time) bool {
		return true // Force refresh
	}

	refreshedToken, err := client.Token(ctx, config)
	require.NoError(t, err, "Token refresh failed")

	assert.NotEmpty(t, refreshedToken.AccessToken, "Refreshed AccessToken should not be empty")

	t.Logf("Token refresh successful, new token expires at %v", refreshedToken.ExpiresAt)
}
