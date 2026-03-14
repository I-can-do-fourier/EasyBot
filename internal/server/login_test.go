package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildAuthorizeURLIncludesRequiredParameters(t *testing.T) {
	state := "state-123"
	challenge := "challenge-456"

	rawURL := buildAuthorizeURL(state, challenge)
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}

	query := parsedURL.Query()
	if parsedURL.Scheme != "https" || parsedURL.Host != "auth.openai.com" {
		t.Fatalf("unexpected authorize URL host: %s", parsedURL.String())
	}
	if got := query.Get("client_id"); got != oauthClientID {
		t.Fatalf("client_id = %q, want %q", got, oauthClientID)
	}
	if got := query.Get("redirect_uri"); got != oauthRedirectURI {
		t.Fatalf("redirect_uri = %q, want %q", got, oauthRedirectURI)
	}
	if got := query.Get("code_challenge"); got != challenge {
		t.Fatalf("code_challenge = %q, want %q", got, challenge)
	}
	if got := query.Get("state"); got != state {
		t.Fatalf("state = %q, want %q", got, state)
	}
	if got := query.Get("scope"); got != oauthScope {
		t.Fatalf("scope = %q, want %q", got, oauthScope)
	}
}

func TestParseAuthorizationCodeInputFromRedirectURL(t *testing.T) {
	code, err := parseAuthorizationCodeInput("http://localhost:1455/auth/callback?code=abc123&state=state-123", "state-123")
	if err != nil {
		t.Fatalf("parseAuthorizationCodeInput returned error: %v", err)
	}
	if code != "abc123" {
		t.Fatalf("code = %q, want %q", code, "abc123")
	}
}

func TestExchangeAuthorizationCodePostsExpectedForm(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
				t.Fatalf("Content-Type = %q, want form encoding", got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll returned error: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("ParseQuery returned error: %v", err)
			}
			if got := values.Get("grant_type"); got != "authorization_code" {
				t.Fatalf("grant_type = %q, want %q", got, "authorization_code")
			}
			if got := values.Get("client_id"); got != oauthClientID {
				t.Fatalf("client_id = %q, want %q", got, oauthClientID)
			}
			if got := values.Get("code"); got != "code-123" {
				t.Fatalf("code = %q, want %q", got, "code-123")
			}
			if got := values.Get("code_verifier"); got != "verifier-123" {
				t.Fatalf("code_verifier = %q, want %q", got, "verifier-123")
			}
			if got := values.Get("redirect_uri"); got != oauthRedirectURI {
				t.Fatalf("redirect_uri = %q, want %q", got, oauthRedirectURI)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"access.jwt","refresh_token":"refresh","id_token":"id.jwt","token_type":"Bearer","expires_in":3600}`)),
			}, nil
		}),
	}

	tokens, err := exchangeAuthorizationCode(context.Background(), client, "https://auth.example.test/oauth/token", "code-123", "verifier-123")
	if err != nil {
		t.Fatalf("exchangeAuthorizationCode returned error: %v", err)
	}
	if tokens.AccessToken != "access.jwt" {
		t.Fatalf("AccessToken = %q, want %q", tokens.AccessToken, "access.jwt")
	}
}

func TestExchangeAPIKeyUsesTokenExchangeGrant(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll returned error: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("ParseQuery returned error: %v", err)
			}
			if got := values.Get("grant_type"); got != oauthTokenExchangeGrantType {
				t.Fatalf("grant_type = %q, want %q", got, oauthTokenExchangeGrantType)
			}
			if got := values.Get("requested_token"); got != oauthRequestedAPIKeyToken {
				t.Fatalf("requested_token = %q, want %q", got, oauthRequestedAPIKeyToken)
			}
			if got := values.Get("subject_token"); got != "id-token" {
				t.Fatalf("subject_token = %q, want %q", got, "id-token")
			}
			if got := values.Get("subject_token_type"); got != oauthIDTokenSubjectTokenType {
				t.Fatalf("subject_token_type = %q, want %q", got, oauthIDTokenSubjectTokenType)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"sk-test"}`)),
			}, nil
		}),
	}

	apiKey, err := exchangeAPIKey(context.Background(), client, "https://auth.example.test/oauth/token", "id-token", "access-token")
	if err != nil {
		t.Fatalf("exchangeAPIKey returned error: %v", err)
	}
	if apiKey != "sk-test" {
		t.Fatalf("apiKey = %q, want %q", apiKey, "sk-test")
	}
}

func TestBuildAuthFileExtractsAccountIDAndExpiry(t *testing.T) {
	expiry := time.Unix(1_700_000_000, 0).UTC()
	tokens := oauthTokenResponse{
		AccessToken: testJWT(t, map[string]any{
			"exp": expiry.Unix(),
		}),
		IDToken: testJWT(t, map[string]any{
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_account_id": "acct_123",
			},
		}),
		TokenType: "Bearer",
	}

	got := buildAuthFile(tokens)
	if got.AccountID != "acct_123" {
		t.Fatalf("AccountID = %q, want %q", got.AccountID, "acct_123")
	}
	if !got.ExpiresAt.Equal(expiry) {
		t.Fatalf("ExpiresAt = %s, want %s", got.ExpiresAt, expiry)
	}
}

func TestExplainAPIKeyExchangeErrorMentionsPlatformOrg(t *testing.T) {
	err := explainAPIKeyExchangeError(errors.New("invalid_subject_token: missing organization_id"))
	if !strings.Contains(err, "Platform organization/project claim") {
		t.Fatalf("unexpected explanation: %q", err)
	}
}

func testJWT(t *testing.T, claims any) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + ".signature"
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
