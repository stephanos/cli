package cliext

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"golang.org/x/oauth2"
)

// OAuthClientConfig contains OAuth client configuration.
type OAuthClientConfig struct {
	// ClientID is the OAuth client ID.
	ClientID string

	// ClientSecret is the OAuth client secret (optional, for confidential clients).
	ClientSecret string

	// TokenURL is the OAuth token endpoint URL.
	TokenURL string

	// AuthURL is the OAuth authorization endpoint URL.
	AuthURL string

	// RequestParams are additional parameters to include in OAuth requests.
	// Common parameters include "audience" for Auth0/Temporal Cloud.
	RequestParams map[string]string

	// Scopes are the requested OAuth scopes.
	Scopes []string
}

// OAuthToken contains the token fields from an OAuth response.
type OAuthToken struct {
	// AccessToken is the current access token.
	AccessToken string

	// RefreshToken is the refresh token for obtaining new access tokens.
	RefreshToken string

	// TokenType is the type of token (usually "Bearer").
	TokenType string

	// ExpiresAt is when the access token expires.
	ExpiresAt time.Time
}

// OAuthConfig contains OAuth client configuration and current token for an extension.
type OAuthConfig struct {
	OAuthClientConfig
	OAuthToken
}

// oauthTokenFromOAuth2 creates an OAuthToken from an oauth2.Token.
func oauthTokenFromOAuth2(token *oauth2.Token) *OAuthToken {
	return &OAuthToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresAt:    token.Expiry,
	}
}

// OAuthClient handles OAuth authentication flows using Device Code Flow.
type OAuthClient struct {
	Options OAuthClientConfig

	// OnVerification is called to handle the verification URL and user code.
	// Default: prints the URL and code to stdout, then opens the browser.
	OnVerification func(verificationURL, userCode string)

	// OnRefreshError is called when a token refresh fails.
	// It receives the error and can return a new token (e.g., by re-authenticating)
	// or return nil, err to propagate the error.
	// Default: re-authenticates on "invalid_grant" errors.
	OnRefreshError func(ctx context.Context, refreshErr error) (*OAuthToken, error)

	// IsTokenExpired checks if the token needs to be refreshed.
	// Receives the token's expiration time and returns true if a refresh is needed.
	// Default: returns true if the token expires within 1 minute.
	IsTokenExpired func(expiresAt time.Time) bool
}

// NewOAuthClient creates a new OAuth client with the given options.
func NewOAuthClient(opts OAuthClientConfig) *OAuthClient {
	var c *OAuthClient
	c = &OAuthClient{
		Options: opts,
		// Default: print verification info and open browser
		OnVerification: func(verificationURL, userCode string) {
			fmt.Printf("Go to %s and enter code: %s\n", verificationURL, userCode)
			_ = openBrowser(verificationURL)
		},
		// Default: re-login on invalid_grant, propagate other errors
		OnRefreshError: func(ctx context.Context, refreshErr error) (*OAuthToken, error) {
			var retrieveErr *oauth2.RetrieveError
			if errors.As(refreshErr, &retrieveErr) && retrieveErr.ErrorCode == "invalid_grant" {
				// Handle one of two cases:
				//   1. Refresh token has expired.
				//   2. Refresh tokens were enabled, but the user has not logged in to receive one yet.
				return c.Login(ctx)
			}
			return nil, refreshErr
		},
		// Default: refresh if token expires within 1 minute
		IsTokenExpired: func(expiresAt time.Time) bool {
			if expiresAt.IsZero() {
				return true
			}
			return time.Now().After(expiresAt.Add(-1 * time.Minute))
		},
	}
	return c
}

func (c *OAuthClient) createOAuth2Config() *oauth2.Config {
	opts := c.Options
	return &oauth2.Config{
		ClientID: opts.ClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:       opts.AuthURL,
			TokenURL:      opts.TokenURL,
			DeviceAuthURL: opts.AuthURL,
		},
		Scopes: opts.Scopes,
	}
}

// Login performs the OAuth Device Code Flow.
// It requests a device code, displays the verification URL and code to the user,
// and polls for the token until the user completes authorization.
func (c *OAuthClient) Login(ctx context.Context) (*OAuthToken, error) {
	cfg := c.createOAuth2Config()

	authOpts := []oauth2.AuthCodeOption{}
	for key, value := range c.Options.RequestParams {
		authOpts = append(authOpts, oauth2.SetAuthURLParam(key, value))
	}

	deviceAuth, err := cfg.DeviceAuth(ctx, authOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to request device code: %w", err)
	}

	verificationURL := deviceAuth.VerificationURIComplete
	if verificationURL == "" {
		verificationURL = deviceAuth.VerificationURI
	}
	c.OnVerification(verificationURL, deviceAuth.UserCode)

	token, err := cfg.DeviceAccessToken(ctx, deviceAuth, authOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	return oauthTokenFromOAuth2(token), nil
}

// Token returns a valid access token, refreshing if necessary.
// It uses IsTokenExpired to determine if the token needs refreshing.
// Updates the config's token fields in place.
func (c *OAuthClient) Token(ctx context.Context, config *OAuthConfig) (*OAuthToken, error) {
	if config == nil || config.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	// Check if token is still valid
	if !c.IsTokenExpired(config.ExpiresAt) {
		return &config.OAuthToken, nil
	}

	// Token is expired or about to expire, refresh it.
	cfg := c.createOAuth2Config()
	oldToken := &oauth2.Token{RefreshToken: config.RefreshToken}
	tokenSource := cfg.TokenSource(ctx, oldToken)
	newToken, err := tokenSource.Token()
	if err != nil {
		if c.OnRefreshError != nil {
			token, handlerErr := c.OnRefreshError(ctx, err)
			if handlerErr != nil {
				return nil, handlerErr
			}
			if token != nil {
				config.OAuthToken = *token
				return token, nil
			}
		}
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	config.OAuthToken = *oauthTokenFromOAuth2(newToken)
	return &config.OAuthToken, nil
}

// openBrowser opens a URL in the default system browser.
func openBrowser(urlStr string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", urlStr)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", urlStr)
	default:
		cmd = exec.Command("xdg-open", urlStr)
	}
	return cmd.Start()
}
