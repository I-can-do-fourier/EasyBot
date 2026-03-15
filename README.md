# EasyBot

`easyBot` is a security-focused local AI assistant for the terminal. It can chat, inspect files inside approved directories, search project content, and run guarded local commands without going through a shell.

The current user-facing workflow is terminal-first and supports two authentication modes: `api_key` and `codex_oauth`.

## What it does

- Runs as an interactive terminal assistant
- Works with OpenAI-compatible models in `api_key` mode
- Supports browser login and saved credentials in `codex_oauth` mode
- Reads, searches, and analyzes local files inside configured roots
- Requires approval for writes and non-safe commands
- Records tool activity in an audit log

## Security model

- Filesystem access is restricted to configured roots.
- Command execution does not invoke a shell.
- Dangerous commands and interpreter-style execution are denied by policy.
- State-changing actions require explicit approval.
- Tool calls are logged for auditing.
- The agent loop uses step and timeout limits.

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
./easybot --model gpt-4.1-mini --roots /Users/you,/tmp
```

## Notes

- Terminal mode is the default runtime.
- The default allowed roots are your home directory and `/tmp`.
- Application logs write to `easybot.log` by default. Set `EASYBOT_APP_LOG` or `--app-log` to move that file, or set `EASYBOT_LOG_LEVEL=off` to disable app logs.
- Built-in tools include file search, text search, PDF text extraction(Not works well right now), and guarded command execution.
