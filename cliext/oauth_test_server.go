package cliext

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"
)

// MockOAuthServer is a minimal mock OAuth server for testing Device Code Flow.
type MockOAuthServer struct {
	Server *httptest.Server

	// Device code response configuration
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               int
	Interval                int

	// Token response configuration
	AccessToken  string
	RefreshToken string
	TokenType    string
	TokenExpiry  int

	// Error configuration - if set, endpoints return errors
	DeviceCodeError       string
	DeviceCodeErrorDesc   string
	TokenError            string
	TokenErrorDescription string

	// Pending controls whether the token endpoint returns "authorization_pending"
	// Set to false to simulate user completing authorization
	Pending bool

	// Request tracking
	LastDeviceCodeRequest url.Values
	LastTokenRequest      url.Values
}

// NewMockOAuthServer creates a simple mock OAuth server for Device Code Flow.
func NewMockOAuthServer() *MockOAuthServer {
	m := &MockOAuthServer{
		DeviceCode:   "mock-device-code",
		UserCode:     "MOCK-CODE",
		ExpiresIn:    600,
		Interval:     1,
		AccessToken:  "mock-access-token",
		RefreshToken: "mock-refresh-token",
		TokenType:    "Bearer",
		TokenExpiry:  3600,
		Pending:      false, // Default: authorization complete
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/device/code", m.handleDeviceCode)
	mux.HandleFunc("/oauth/token", m.handleToken)

	m.Server = httptest.NewServer(mux)

	// Set verification URI to point to mock server
	m.VerificationURI = m.Server.URL + "/activate"
	m.VerificationURIComplete = m.Server.URL + "/activate?user_code=" + m.UserCode

	return m
}

// Close shuts down the mock server.
func (m *MockOAuthServer) Close() {
	if m.Server != nil {
		m.Server.Close()
	}
}

// Domain returns the server's domain for use in OAuthLoginOptions.
func (m *MockOAuthServer) Domain() string {
	return m.Server.URL
}

// DeviceAuthURL returns the device authorization endpoint URL.
func (m *MockOAuthServer) DeviceAuthURL() string {
	return m.Server.URL + "/oauth/device/code"
}

// TokenURL returns the token endpoint URL.
func (m *MockOAuthServer) TokenURL() string {
	return m.Server.URL + "/oauth/token"
}

// handleDeviceCode handles the /oauth/device/code endpoint.
func (m *MockOAuthServer) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		m.writeError(w, "invalid_request", "failed to parse form")
		return
	}

	m.LastDeviceCodeRequest = r.Form

	if m.DeviceCodeError != "" {
		m.writeError(w, m.DeviceCodeError, m.DeviceCodeErrorDesc)
		return
	}

	response := map[string]interface{}{
		"device_code":               m.DeviceCode,
		"user_code":                 m.UserCode,
		"verification_uri":          m.VerificationURI,
		"verification_uri_complete": m.VerificationURIComplete,
		"expires_in":                m.ExpiresIn,
		"interval":                  m.Interval,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleToken handles the /oauth/token endpoint.
func (m *MockOAuthServer) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		m.writeError(w, "invalid_request", "failed to parse form")
		return
	}

	m.LastTokenRequest = r.Form

	// Return configured error
	if m.TokenError != "" {
		m.writeError(w, m.TokenError, m.TokenErrorDescription)
		return
	}

	// For device code grant, check if authorization is pending
	grantType := r.FormValue("grant_type")
	if grantType == "urn:ietf:params:oauth:grant-type:device_code" && m.Pending {
		m.writeError(w, "authorization_pending", "The authorization request is still pending")
		return
	}

	// Return success response
	response := map[string]interface{}{
		"access_token":  m.AccessToken,
		"refresh_token": m.RefreshToken,
		"token_type":    m.TokenType,
		"expires_in":    m.TokenExpiry,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (m *MockOAuthServer) writeError(w http.ResponseWriter, errCode, errDesc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": errDesc,
	})
}

// MockTokenResponse is a helper to create expected token responses for assertions.
func MockTokenResponse(accessToken, refreshToken string, expiresIn int) *OAuthToken {
	return &OAuthToken{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}
}
