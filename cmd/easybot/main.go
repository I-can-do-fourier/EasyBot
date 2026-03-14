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

	"easybot/internal/acp"
	"easybot/internal/agent"
	"easybot/internal/server"
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
		default:
			return "", "", cfg, fmt.Errorf("unknown subcommand %q", args[0])
		}
	}

	fs, values := newFlagSet(cfg, runMode, os.Stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		return "", "", cfg, err
	}

	cfg.BaseURL = strings.TrimSpace(*values.baseURL)
	cfg.APIKey = strings.TrimSpace(*values.apiKey)
	cfg.Model = strings.TrimSpace(*values.model)
	cfg.AllowedRoots = splitAndTrim(*values.roots)
	cfg.MaxSteps = *values.maxSteps
	cfg.StepTimeout = time.Duration(*values.stepTimeoutSec) * time.Second
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
	} else if runMode != "http" && runMode != "acp" {
		runMode = "terminal"
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
	roots          *string
	maxSteps       *int
	stepTimeoutSec *int
	auditLog       *string
	outputMaxBytes *int
	acpMode        *bool
}

func newFlagSet(cfg agent.Config, runMode string, output io.Writer) (*flag.FlagSet, flagValues) {
	fs := flag.NewFlagSet("easybot", flag.ContinueOnError)
	fs.SetOutput(output)
	values := flagValues{
		httpMode:       fs.Bool("http", runMode == "http", "run HTTP server mode"),
		listen:         fs.String("listen", ":8080", "HTTP listen address"),
		baseURL:        fs.String("base-url", cfg.BaseURL, "LLM API base URL"),
		apiKey:         fs.String("api-key", cfg.APIKey, "LLM API key"),
		model:          fs.String("model", cfg.Model, "LLM model name"),
		roots:          fs.String("roots", strings.Join(cfg.AllowedRoots, ","), "comma-separated allowed roots"),
		maxSteps:       fs.Int("max-steps", cfg.MaxSteps, "maximum agent loop steps"),
		stepTimeoutSec: fs.Int("step-timeout-sec", int(cfg.StepTimeout/time.Second), "per-step timeout in seconds"),
		auditLog:       fs.String("audit-log", cfg.AuditLogPath, "audit log path"),
		outputMaxBytes: fs.Int("tool-output-max-bytes", cfg.ToolOutputMaxBytes, "max bytes captured from each tool output stream"),
		acpMode:        fs.Bool("acp", cfg.Mode == agent.ModeACP, "run ACP stdio mode"),
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
