# EasyBot

`easyBot` is a light-weight local AI assistant for the terminal. It can chat, read and edit files inside approved directories, work across codebases and local projects, search project content, and run guarded local commands without going through a shell.

The current user-facing workflow is terminal-first and supports two authentication modes: `api_key` and `codex_oauth`.

## What it does

- Runs as an interactive terminal assistant
- Works with OpenAI-compatible models in `api_key` mode
- Supports browser login and saved credentials in `codex_oauth` mode
- Reads, edits, searches, and analyzes local files, code, and projects inside configured roots
- Includes basic safety controls for file changes and command execution

## Safety

`easyBot` includes safety controls such as restricted working roots, approval for state-changing actions, and guarded local command execution.

## Quick start

Build the binary:

```bash
make build
```

Run in terminal mode:

```bash
./easybot
```

You can also run it without building:

```bash
go run ./cmd/easybot
```

## Command line usage

```bash
./easybot --auth-mode api_key --model gpt-4.1-mini --base-url https://api.openai.com/v1 --api-key your_api_key
./easybot login
./easybot --auth-mode codex_oauth --model gpt-5.4
./easybot --auth-mode api_key --model gpt-4.1-mini --roots /path/to/project,/tmp
./easybot --help
```

- `./easybot --auth-mode api_key ...` starts the terminal assistant with API key authentication.
- `./easybot login` starts the OAuth login flow and stores credentials for `codex_oauth`.
- `./easybot --auth-mode codex_oauth ...` starts the terminal assistant with saved OAuth credentials.
- `./easybot --auth-mode api_key --model ... --roots ...` overrides the auth mode, model, and allowed roots for the current run.
- `./easybot --help` shows the available CLI flags.

## Authentication

Set one of the following auth modes before starting `easyBot`.

### `api_key`

Use an OpenAI-compatible base URL plus an API key:

```bash
export EASYBOT_AUTH_MODE=api_key
export EASYBOT_MODEL=gpt-4.1-mini
export EASYBOT_BASE_URL=https://api.openai.com/v1
export EASYBOT_API_KEY=your_api_key
```

### `codex_oauth`

Use the Codex OAuth login flow and saved credentials:

```bash
export EASYBOT_AUTH_MODE=codex_oauth
export EASYBOT_MODEL=gpt-5.4
```

Then sign in interactively:

```bash
go run ./cmd/easybot login
```

The login flow stores credentials and runtime defaults in `~/.easybot/auth.json`. When related environment variables are unset, `easyBot` reuses the saved `auth_mode`, `base_url`, `model`, `access_token`, and `account_id`.

## Common configuration

Optional environment variables:

```bash
export EASYBOT_ALLOWED_ROOTS=/Users/you,/tmp
export EASYBOT_MAX_STEPS=8
export EASYBOT_STEP_TIMEOUT_SEC=45
export EASYBOT_LOG_LEVEL=info
export EASYBOT_APP_LOG=./easybot.log
export EASYBOT_AUDIT_LOG=/tmp/easybot-audit.jsonl
export EASYBOT_TOOL_OUTPUT_MAX_BYTES=65536
```

You can also override settings with flags:

```bash
./easybot --auth-mode api_key --model gpt-4.1-mini --roots /Users/you,/tmp
```

## Notes

- Terminal mode is the default runtime.
- The default allowed roots are your home directory and `/tmp`.
- Application logs write to `easybot.log` by default. Set `EASYBOT_APP_LOG` or `--app-log` to move that file, or set `EASYBOT_LOG_LEVEL=off` to disable app logs.
- Built-in tools include file search, text search, PDF text extraction, and guarded command execution.
- PDF text extraction is available, but it does not work well yet.
