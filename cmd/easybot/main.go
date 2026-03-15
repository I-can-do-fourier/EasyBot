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

var hiddenUsageFlags = map[string]struct{}{
	"acp":    {},
	"http":   {},
	"listen": {},
}

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
	runMode := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "terminal", "http", "acp":
			runMode = args[0]
			args = args[1:]
		case "help", "--help", "-h":
			fs, _ := newFlagSet(runMode, os.Stdout)
			fs.Usage()
			os.Exit(0)
		case "login", "--login":
			runMode = "login"
			args = args[1:]
		default:
			fs, _ := newFlagSet(runMode, os.Stdout)
			fs.Usage()
			return "", "", cfg, fmt.Errorf("unknown subcommand %q", args[0])
		}
	}

	fs, values := newFlagSet(runMode, os.Stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fs.Usage()
		return "", "", cfg, err
	}

	setFlags := make(map[string]struct{})
	fs.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = struct{}{}
	})

	if _, ok := setFlags["base-url"]; ok {
		cfg.BaseURL = strings.TrimSpace(*values.baseURL)
	}
	if _, ok := setFlags["api-key"]; ok {
		cfg.APIKey = strings.TrimSpace(*values.apiKey)
	}
	if _, ok := setFlags["access-token"]; ok {
		cfg.AccessToken = strings.TrimSpace(*values.accessToken)
	}
	if _, ok := setFlags["account-id"]; ok {
		cfg.AccountID = strings.TrimSpace(*values.accountID)
	}
	if _, ok := setFlags["model"]; ok {
		cfg.Model = strings.TrimSpace(*values.model)
	}
	if _, ok := setFlags["auth-mode"]; ok {
		cfg.AuthMode = agent.AuthMode(strings.TrimSpace(*values.authMode))
		if cfg.AuthMode == "" {
			cfg.AuthMode = agent.AuthModeAPIKey
		}
	}
	if _, ok := setFlags["roots"]; ok {
		cfg.AllowedRoots = splitAndTrim(*values.roots)
	}
	if _, ok := setFlags["max-steps"]; ok {
		cfg.MaxSteps = *values.maxSteps
	}
	if _, ok := setFlags["step-timeout-sec"]; ok {
		cfg.StepTimeout = time.Duration(*values.stepTimeoutSec) * time.Second
	}
	if _, ok := setFlags["app-log"]; ok {
		cfg.AppLogPath = strings.TrimSpace(*values.appLog)
	}
	if _, ok := setFlags["audit-log"]; ok {
		cfg.AuditLogPath = strings.TrimSpace(*values.auditLog)
	}
	if _, ok := setFlags["tool-output-max-bytes"]; ok {
		cfg.ToolOutputMaxBytes = *values.outputMaxBytes
	}

	if runMode == "" {
		if cfg.Mode == agent.ModeACP {
			runMode = "acp"
		} else {
			runMode = "terminal"
		}
	}

	if _, ok := setFlags["acp"]; ok && *values.acpMode {
		runMode = "acp"
	}
	if _, ok := setFlags["http"]; ok && *values.httpMode {
		if runMode == "acp" {
			return "", "", cfg, fmt.Errorf("--http and --acp cannot be used together")
		}
		runMode = "http"
	}
	if _, ok := setFlags["login"]; ok && *values.loginMode {
		runMode = "login"
	}
	if runMode == "acp" {
		cfg.Mode = agent.ModeACP
	} else if runMode != "login" {
		cfg.Mode = agent.ModeNonACP
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

func newFlagSet(runMode string, output io.Writer) (*flag.FlagSet, flagValues) {
	fs := flag.NewFlagSet("easybot", flag.ContinueOnError)
	fs.SetOutput(output)
	defaultCfg := agent.DefaultConfig()
	values := flagValues{
		httpMode:       fs.Bool("http", runMode == "http", "run HTTP server mode"),
		listen:         fs.String("listen", ":8080", "HTTP listen address"),
		baseURL:        fs.String("base-url", defaultCfg.BaseURL, "LLM API base URL"),
		apiKey:         fs.String("api-key", "", "LLM API key"),
		accessToken:    fs.String("access-token", "", "OAuth access token for Codex auth mode"),
		accountID:      fs.String("account-id", "", "ChatGPT/Codex account ID for Codex auth mode"),
		model:          fs.String("model", defaultCfg.Model, "LLM model name"),
		authMode:       fs.String("auth-mode", string(defaultCfg.AuthMode), "auth mode: api_key or codex_oauth"),
		roots:          fs.String("roots", strings.Join(defaultCfg.AllowedRoots, ","), "comma-separated allowed roots"),
		maxSteps:       fs.Int("max-steps", defaultCfg.MaxSteps, "maximum agent loop steps"),
		stepTimeoutSec: fs.Int("step-timeout-sec", int(defaultCfg.StepTimeout/time.Second), "per-step timeout in seconds"),
		appLog:         fs.String("app-log", defaultCfg.AppLogPath, "application log path"),
		auditLog:       fs.String("audit-log", defaultCfg.AuditLogPath, "audit log path"),
		outputMaxBytes: fs.Int("tool-output-max-bytes", defaultCfg.ToolOutputMaxBytes, "max bytes captured from each tool output stream"),
		acpMode:        fs.Bool("acp", runMode == "acp", "run ACP stdio mode"),
		loginMode:      fs.Bool("login", false, "run login mode to get oauth token interactively"),
	}
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), `easyBot

	Run in terminal mode:
  ./easybot

	Run interactive login:
  ./easybot login

Flags:
`)
		printVisibleDefaults(fs)
	}
	return fs, values
}

func printVisibleDefaults(fs *flag.FlagSet) {
	fs.VisitAll(func(f *flag.Flag) {
		if _, hidden := hiddenUsageFlags[f.Name]; hidden {
			return
		}
		name, usage := flag.UnquoteUsage(f)
		if name != "" {
			fmt.Fprintf(fs.Output(), "  -%s %s\n", f.Name, name)
		} else {
			fmt.Fprintf(fs.Output(), "  -%s\n", f.Name)
		}
		if defValue, ok := usageDefaultValue(f); ok {
			fmt.Fprintf(fs.Output(), "    %s (default %q)\n", usage, defValue)
			return
		}
		fmt.Fprintf(fs.Output(), "    %s\n", usage)
	})
}

func usageDefaultValue(f *flag.Flag) (string, bool) {
	if boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && boolFlag.IsBoolFlag() {
		if f.DefValue == "false" {
			return "", false
		}
		return f.DefValue, true
	}
	if f.DefValue == "" {
		return "", false
	}
	return f.DefValue, true
}
