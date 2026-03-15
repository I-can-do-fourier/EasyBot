package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/I-can-do-fourier/EasyBot/internal/acp"
	"github.com/I-can-do-fourier/EasyBot/internal/agent"
	"github.com/I-can-do-fourier/EasyBot/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := agent.LoadConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	runMode, listen, cfg, err := parseArgs(cfg, os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	switch runMode {
	case "terminal":
		if err := agent.RunTerminal(ctx, cfg, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
	case "http":
		if err := server.Run(ctx, listen, cfg); err != nil {
			log.Fatal(err)
		}
	case "acp":
		if err := acp.Serve(ctx, cfg, os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
	case "login":
		// run a temporary HTTP server to handle the OAuth callback and store the to over run.sh file
		if err := server.RunLogin(ctx, cfg); err != nil {
			log.Fatal(err)
		}
		/* this is the codex for the login flow that we want to implement in server.RunLogin:
		generate PKCE verifier/challenge + random state
		open https://auth.openai.com/oauth/authorize?...
		try to capture callback on http://127.0.0.1:1455/auth/callback
		if callback can’t bind (or you’re remote/headless), paste the redirect URL/code
		exchange at https://auth.openai.com/oauth/token
		extract accountId from the access token and store { access, refresh, expires, accountId }

		*/

		// 1. tell the user that they need to login via the browser

		// 2. open the browser to the login page

		// 3. after successful login, the server will receive the callback, exchange the code for a token, and print out instructions on how to the config file

		// 4. the user can then copy the token and set it as an environment variable or use it in their application

	default:
		log.Fatalf("unknown mode %q", runMode)
	}
}

func parseArgs(cfg agent.Config, args []string) (string, string, agent.Config, error) {
	runMode := "terminal"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "terminal", "http", "acp":
			runMode = args[0]
			args = args[1:]
		case "help", "--help", "-h":
			fs, _ := newFlagSet(cfg, runMode, os.Stdout)
			fs.Usage()
			os.Exit(0)
		case "login", "--login":
			runMode = "login"
			args = args[1:]
		default:
			fs, _ := newFlagSet(cfg, runMode, os.Stdout)
			fs.Usage()
			return "", "", cfg, fmt.Errorf("unknown subcommand %q", args[0])
		}
	}

	fs, values := newFlagSet(cfg, runMode, os.Stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fs.Usage()
		return "", "", cfg, err
	}

	cfg.BaseURL = strings.TrimSpace(*values.baseURL)
	cfg.APIKey = strings.TrimSpace(*values.apiKey)
	cfg.AccessToken = strings.TrimSpace(*values.accessToken)
	cfg.AccountID = strings.TrimSpace(*values.accountID)
	cfg.Model = strings.TrimSpace(*values.model)
	cfg.AuthMode = agent.AuthMode(strings.TrimSpace(*values.authMode))
	if cfg.AuthMode == "" {
		cfg.AuthMode = agent.AuthModeAPIKey
	}
	cfg.AllowedRoots = splitAndTrim(*values.roots)
	cfg.MaxSteps = *values.maxSteps
	cfg.StepTimeout = time.Duration(*values.stepTimeoutSec) * time.Second
	cfg.AppLogPath = strings.TrimSpace(*values.appLog)
	cfg.AuditLogPath = strings.TrimSpace(*values.auditLog)
	cfg.ToolOutputMaxBytes = *values.outputMaxBytes
	if *values.acpMode {
		runMode = "acp"
		cfg.Mode = agent.ModeACP
	} else {
		cfg.Mode = agent.ModeNonACP
	}

	if *values.httpMode {
		if runMode == "acp" {
			return "", "", cfg, fmt.Errorf("--http and --acp cannot be used together")
		}
		runMode = "http"
	} else if runMode != "http" && runMode != "acp" && runMode != "login" {
		runMode = "terminal"
	}
	if *values.loginMode {
		runMode = "login"
	}
	if len(cfg.AllowedRoots) == 0 {
		return "", "", cfg, fmt.Errorf("at least one allowed root is required")
	}
	if cfg.MaxSteps <= 0 {
		return "", "", cfg, fmt.Errorf("--max-steps must be > 0")
	}
	if cfg.StepTimeout <= 0 {
		return "", "", cfg, fmt.Errorf("--step-timeout-sec must be > 0")
	}
	if cfg.ToolOutputMaxBytes < 4096 {
		return "", "", cfg, fmt.Errorf("--tool-output-max-bytes must be >= 4096")
	}
	if cfg.AuthMode != agent.AuthModeAPIKey && cfg.AuthMode != agent.AuthModeCodex {
		return "", "", cfg, fmt.Errorf("--auth-mode must be %q or %q", agent.AuthModeAPIKey, agent.AuthModeCodex)
	}
	return runMode, *values.listen, cfg, nil
}

func splitAndTrim(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

type flagValues struct {
	httpMode       *bool
	listen         *string
	baseURL        *string
	apiKey         *string
	model          *string
	authMode       *string
	accessToken    *string
	accountID      *string
	roots          *string
	maxSteps       *int
	stepTimeoutSec *int
	appLog         *string
	auditLog       *string
	outputMaxBytes *int
	acpMode        *bool
	loginMode      *bool
}

func newFlagSet(cfg agent.Config, runMode string, output io.Writer) (*flag.FlagSet, flagValues) {
	fs := flag.NewFlagSet("easybot", flag.ContinueOnError)
	fs.SetOutput(output)
	authMode := string(cfg.AuthMode)
	if strings.TrimSpace(authMode) == "" {
		authMode = string(agent.AuthModeAPIKey)
	}
	values := flagValues{
		httpMode:       fs.Bool("http", runMode == "http", "run HTTP server mode"),
		listen:         fs.String("listen", ":8080", "HTTP listen address"),
		baseURL:        fs.String("base-url", cfg.BaseURL, "LLM API base URL"),
		apiKey:         fs.String("api-key", cfg.APIKey, "LLM API key"),
		accessToken:    fs.String("access-token", cfg.AccessToken, "OAuth access token for Codex auth mode"),
		accountID:      fs.String("account-id", cfg.AccountID, "ChatGPT/Codex account ID for Codex auth mode"),
		model:          fs.String("model", cfg.Model, "LLM model name"),
		authMode:       fs.String("auth-mode", authMode, "auth mode: api_key or codex_oauth"),
		roots:          fs.String("roots", strings.Join(cfg.AllowedRoots, ","), "comma-separated allowed roots"),
		maxSteps:       fs.Int("max-steps", cfg.MaxSteps, "maximum agent loop steps"),
		stepTimeoutSec: fs.Int("step-timeout-sec", int(cfg.StepTimeout/time.Second), "per-step timeout in seconds"),
		appLog:         fs.String("app-log", cfg.AppLogPath, "application log path"),
		auditLog:       fs.String("audit-log", cfg.AuditLogPath, "audit log path"),
		outputMaxBytes: fs.Int("tool-output-max-bytes", cfg.ToolOutputMaxBytes, "max bytes captured from each tool output stream"),
		acpMode:        fs.Bool("acp", cfg.Mode == agent.ModeACP, "run ACP stdio mode"),
		loginMode:      fs.Bool("login", false, "run login mode to get oauth token interactively"),
	}
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `easyBot

	Run directly in terminal mode:
  ./easybot

	Run HTTP mode:
  ./easybot --http --listen :8080

Run ACP stdio mode:
  ./easybot acp
  ./easybot --acp

Flags:
`)
		fs.PrintDefaults()
	}
	return fs, values
}
