package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"easybot/internal/agent"
	"easybot/internal/auth"
)

const (
	oauthAuthorizeURL            = "https://auth.openai.com/oauth/authorize"
	oauthTokenURL                = "https://auth.openai.com/oauth/token"
	oauthClientID                = "app_EMoamEEZ73f0CkXaXp7hrann"
	oauthRedirectURI             = "http://localhost:1455/auth/callback"
	oauthCallbackListenAddr      = "127.0.0.1:1455"
	oauthCallbackPath            = "/auth/callback"
	oauthScope                   = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	oauthTokenExchangeGrantType  = "urn:ietf:params:oauth:grant-type:token-exchange"
	oauthIDTokenSubjectTokenType = "urn:ietf:params:oauth:token-type:id_token"
	oauthAccessTokenSubjectType  = "urn:ietf:params:oauth:token-type:access_token"
	oauthRequestedAPIKeyToken    = "openai-api-key"
	manualLoginPromptDelay       = 15 * time.Second
	defaultOpenAIBaseURL         = "https://api.openai.com/v1"
	defaultCodexBaseURL          = "https://chatgpt.com/backend-api/codex"
	defaultOpenAIModel           = "gpt-5.4"
	loginUserAgent               = "easybot/0.1"
)

type oauthTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type oauthAPIKeyResponse struct {
	AccessToken      string `json:"access_token"`
	Token            string `json:"token"`
	APIKey           string `json:"api_key"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type jwtClaims struct {
	ChatGPTAccountID string `json:"chatgpt_account_id"`
	AccountID        string `json:"account_id"`
	OrganizationID   string `json:"organization_id"`
	Exp              int64  `json:"exp"`
}

type oauthCallbackResult struct {
	Code string
	Err  error
}

func RunLogin(ctx context.Context, cfg agent.Config) error {
	codeVerifier, err := randomURLSafeString(48)
	if err != nil {
		return fmt.Errorf("generate PKCE verifier: %w", err)
	}
	state, err := randomURLSafeString(24)
	if err != nil {
		return fmt.Errorf("generate OAuth state: %w", err)
	}

	authorizeURL := buildAuthorizeURL(state, pkceChallenge(codeVerifier))
	httpClient := &http.Client{Timeout: 20 * time.Second}

	fmt.Println("OpenAI login is required in the browser.")
	fmt.Println("A local callback listener will try to capture the redirect on http://127.0.0.1:1455/auth/callback.")
	fmt.Println("If that cannot work on this machine, paste the final redirect URL or the raw authorization code back into the terminal.")
	fmt.Println("")
	fmt.Println("Login URL:")
	fmt.Println(authorizeURL)
	fmt.Println("")

	callbackCh := make(chan oauthCallbackResult, 1)
	serverErrCh := make(chan error, 1)
	stopManualPrompt := make(chan struct{})
	stopServer := func(context.Context) error { return nil }

	listener, err := net.Listen("tcp", oauthCallbackListenAddr)
	if err != nil {
		fmt.Printf("Could not bind %s for automatic callback capture: %v\n", oauthCallbackListenAddr, err)
		fmt.Println("Continuing with manual paste mode.")
		fmt.Println("")
	} else {
		stopServer = startOAuthCallbackServer(listener, state, callbackCh, serverErrCh)
		fmt.Printf("Waiting for the browser redirect on %s ...\n\n", oauthRedirectURI)
	}

	if err := openBrowser(authorizeURL); err != nil {
		fmt.Printf("Browser could not be opened automatically: %v\n", err)
		fmt.Println("Open the login URL manually in your browser.")
		fmt.Println("")
	}

	manualCh := make(chan oauthCallbackResult, 1)
	delay := manualLoginPromptDelay
	if listener == nil {
		delay = 0
	}
	go promptForManualCode(stopManualPrompt, delay, state, manualCh)

	var authCode string
	select {
	case <-ctx.Done():
		close(stopManualPrompt)
		_ = shutdownLoginServer(stopServer)
		return ctx.Err()
	case err := <-serverErrCh:
		close(stopManualPrompt)
		_ = shutdownLoginServer(stopServer)
		return fmt.Errorf("run OAuth callback server: %w", err)
	case result := <-callbackCh:
		close(stopManualPrompt)
		_ = shutdownLoginServer(stopServer)
		if result.Err != nil {
			return result.Err
		}
		authCode = result.Code
	case result := <-manualCh:
		close(stopManualPrompt)
		_ = shutdownLoginServer(stopServer)
		if result.Err != nil {
			return result.Err
		}
		authCode = result.Code
	}

	tokens, err := exchangeAuthorizationCode(ctx, httpClient, oauthTokenURL, authCode, codeVerifier)
	if err != nil {
		return err
	}

	saved := buildAuthFile(tokens)
	if saved.ExpiresAt.IsZero() && tokens.ExpiresIn > 0 {
		saved.ExpiresAt = time.Now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	}
	saved.BaseURL = defaultCodexBaseURL
	saved.Model = defaultOpenAIModel
	if strings.TrimSpace(cfg.Model) != "" {
		saved.Model = cfg.Model
	}
	saved.AuthMode = string(agent.AuthModeCodex)

	// apiKey, apiKeyErr := exchangeAPIKey(ctx, httpClient, oauthTokenURL, tokens.IDToken, tokens.AccessToken)
	// if apiKeyErr == nil {
	// 	saved.APIKey = apiKey
	// }

	authPath, err := auth.Save(saved)
	if err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("Login succeeded.")
	fmt.Printf("Saved OAuth tokens to %s\n", authPath)
	if saved.AccountID != "" {
		fmt.Printf("Account ID: %s\n", saved.AccountID)
	}
	if !saved.ExpiresAt.IsZero() {
		fmt.Printf("Access token expires at %s\n", saved.ExpiresAt.UTC().Format(time.RFC3339))
	}
	fmt.Printf("Saved runtime defaults: auth_mode=%s, base_url=%s, model=%s\n", saved.AuthMode, saved.BaseURL, saved.Model)
	if saved.APIKey != "" {
		fmt.Println("An OpenAI API key was also exchanged and saved. easyBot will reuse it automatically when EASYBOT_API_KEY is unset.")
	} else {
		fmt.Println("OAuth login completed and the raw OAuth bundle was saved.")
		fmt.Println("easyBot will use the saved Codex OAuth config when EASYBOT_AUTH_MODE/EASYBOT_BASE_URL/EASYBOT_ACCESS_TOKEN are not overridden.")
	}
	if cfg.BaseURL != defaultOpenAIBaseURL || !strings.HasPrefix(cfg.Model, "gpt-") {
		fmt.Println("Current runtime settings differ from the saved Codex OAuth defaults.")
		fmt.Printf("Use EASYBOT_BASE_URL=%s and a GPT model such as %s if you want to override the saved login config.\n", saved.BaseURL, saved.Model)
	}

	return nil
}

func buildAuthorizeURL(state, codeChallenge string) string {
	query := url.Values{}
	query.Set("client_id", oauthClientID)
	query.Set("code_challenge", codeChallenge)
	query.Set("code_challenge_method", "S256")
	query.Set("codex_cli_simplified_flow", "true")
	query.Set("id_token_add_organizations", "true")
	query.Set("originator", "codex_vscode")
	query.Set("redirect_uri", oauthRedirectURI)
	query.Set("response_type", "code")
	query.Set("scope", oauthScope)
	query.Set("screen_hint", "login")
	query.Set("state", state)
	return oauthAuthorizeURL + "?" + query.Encode()
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomURLSafeString(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func startOAuthCallbackServer(listener net.Listener, expectedState string, resultCh chan<- oauthCallbackResult, serverErrCh chan<- error) func(context.Context) error {
	var once sync.Once
	sendResult := func(result oauthCallbackResult) {
		once.Do(func() {
			resultCh <- result
		})
	}

	mux := http.NewServeMux()
	mux.HandleFunc(oauthCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if oauthErr := strings.TrimSpace(query.Get("error")); oauthErr != "" {
			message := strings.TrimSpace(query.Get("error_description"))
			if message == "" {
				message = oauthErr
			} else {
				message = oauthErr + ": " + message
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "easyBot login failed. Return to the terminal for details.\n")
			sendResult(oauthCallbackResult{Err: errors.New(message)})
			return
		}

		if gotState := strings.TrimSpace(query.Get("state")); gotState != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "easyBot login failed: OAuth state mismatch.\n")
			sendResult(oauthCallbackResult{Err: errors.New("OAuth state mismatch in callback")})
			return
		}

		code := strings.TrimSpace(query.Get("code"))
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "easyBot login failed: missing authorization code.\n")
			sendResult(oauthCallbackResult{Err: errors.New("callback did not include an authorization code")})
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><html><body><h1>easyBot login complete</h1><p>You can return to the terminal.</p></body></html>")
		sendResult(oauthCallbackResult{Code: code})
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case serverErrCh <- err:
			default:
			}
		}
	}()

	return srv.Shutdown
}

func shutdownLoginServer(stop func(context.Context) error) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return stop(shutdownCtx)
}

func promptForManualCode(done <-chan struct{}, delay time.Duration, expectedState string, resultCh chan<- oauthCallbackResult) {
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-done:
			return
		case <-timer.C:
		}
	}

	select {
	case <-done:
		return
	default:
	}

	fmt.Println("Paste the final redirect URL or the raw authorization code, then press Enter:")
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		select {
		case <-done:
		case resultCh <- oauthCallbackResult{Err: err}:
		}
		return
	}

	code, parseErr := parseAuthorizationCodeInput(strings.TrimSpace(text), expectedState)
	select {
	case <-done:
	case resultCh <- oauthCallbackResult{Code: code, Err: parseErr}:
	}
}

func parseAuthorizationCodeInput(input, expectedState string) (string, error) {
	if input == "" {
		return "", errors.New("no redirect URL or authorization code was provided")
	}

	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		parsedURL, err := url.Parse(input)
		if err != nil {
			return "", fmt.Errorf("parse redirect URL: %w", err)
		}

		query := parsedURL.Query()
		if oauthErr := strings.TrimSpace(query.Get("error")); oauthErr != "" {
			message := strings.TrimSpace(query.Get("error_description"))
			if message == "" {
				return "", errors.New(oauthErr)
			}
			return "", fmt.Errorf("%s: %s", oauthErr, message)
		}

		if gotState := strings.TrimSpace(query.Get("state")); expectedState != "" && gotState != expectedState {
			return "", errors.New("OAuth state mismatch in pasted redirect URL")
		}

		code := strings.TrimSpace(query.Get("code"))
		if code == "" {
			return "", errors.New("redirect URL did not include an authorization code")
		}
		return code, nil
	}

	return input, nil
}

func exchangeAuthorizationCode(ctx context.Context, httpClient *http.Client, endpoint, code, codeVerifier string) (oauthTokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", oauthClientID)
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	form.Set("redirect_uri", oauthRedirectURI)

	var tokens oauthTokenResponse
	if err := postOAuthForm(ctx, httpClient, endpoint, form, &tokens); err != nil {
		return oauthTokenResponse{}, err
	}

	if tokens.Error != "" {
		if tokens.ErrorDescription != "" {
			return oauthTokenResponse{}, fmt.Errorf("%s: %s", tokens.Error, tokens.ErrorDescription)
		}
		return oauthTokenResponse{}, errors.New(tokens.Error)
	}
	if tokens.AccessToken == "" {
		return oauthTokenResponse{}, errors.New("token response did not include an access token")
	}

	return tokens, nil
}

func exchangeAPIKey(ctx context.Context, httpClient *http.Client, endpoint, idToken, accessToken string) (string, error) {
	subjectToken := strings.TrimSpace(idToken)
	subjectTokenType := oauthIDTokenSubjectTokenType
	if subjectToken == "" {
		subjectToken = strings.TrimSpace(accessToken)
		subjectTokenType = oauthAccessTokenSubjectType
	}
	if subjectToken == "" {
		return "", errors.New("no OAuth token available for API key exchange")
	}

	form := url.Values{}
	form.Set("grant_type", oauthTokenExchangeGrantType)
	form.Set("client_id", oauthClientID)
	form.Set("requested_token", oauthRequestedAPIKeyToken)
	form.Set("subject_token", subjectToken)
	form.Set("subject_token_type", subjectTokenType)

	var response oauthAPIKeyResponse
	if err := postOAuthForm(ctx, httpClient, endpoint, form, &response); err != nil {
		return "", err
	}
	if response.Error != "" {
		if response.ErrorDescription != "" {
			return "", fmt.Errorf("%s: %s", response.Error, response.ErrorDescription)
		}
		return "", errors.New(response.Error)
	}

	apiKey := strings.TrimSpace(response.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(response.Token)
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(response.AccessToken)
	}
	if apiKey == "" {
		return "", errors.New("token exchange response did not include a usable API key")
	}

	return apiKey, nil
}

func postOAuthForm(ctx context.Context, httpClient *http.Client, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", loginUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(raw, out); err != nil {
		if resp.StatusCode >= 300 {
			return fmt.Errorf("OAuth server returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
		}
		return fmt.Errorf("decode OAuth response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return fmt.Errorf("OAuth server returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return nil
}

func buildAuthFile(tokens oauthTokenResponse) auth.File {
	accessClaims, _ := decodeJWTClaims(tokens.AccessToken)
	idClaims, _ := decodeJWTClaims(tokens.IDToken)

	accountID := firstNonEmpty(idClaims.ChatGPTAccountID, idClaims.AccountID, accessClaims.ChatGPTAccountID, accessClaims.AccountID)
	expiresAt := time.Time{}
	if accessClaims.Exp > 0 {
		expiresAt = time.Unix(accessClaims.Exp, 0).UTC()
	} else if idClaims.Exp > 0 {
		expiresAt = time.Unix(idClaims.Exp, 0).UTC()
	}

	return auth.File{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IDToken:      tokens.IDToken,
		TokenType:    tokens.TokenType,
		AccountID:    accountID,
		ExpiresAt:    expiresAt,
	}
}

func decodeJWTClaims(token string) (jwtClaims, error) {
	if strings.TrimSpace(token) == "" {
		return jwtClaims{}, errors.New("missing JWT")
	}

	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return jwtClaims{}, errors.New("token is not a JWT")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, fmt.Errorf("decode JWT payload: %w", err)
	}

	var claims struct {
		ChatGPTAccountID string     `json:"chatgpt_account_id"`
		AccountID        string     `json:"account_id"`
		OrganizationID   string     `json:"organization_id"`
		Exp              int64      `json:"exp"`
		OpenAIAuth       *jwtClaims `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return jwtClaims{}, fmt.Errorf("decode JWT claims: %w", err)
	}

	if claims.OpenAIAuth != nil {
		nested := *claims.OpenAIAuth
		if nested.Exp == 0 {
			nested.Exp = claims.Exp
		}
		return nested, nil
	}

	return jwtClaims{
		ChatGPTAccountID: claims.ChatGPTAccountID,
		AccountID:        claims.AccountID,
		OrganizationID:   claims.OrganizationID,
		Exp:              claims.Exp,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func openBrowser(targetURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	return cmd.Start()
}
