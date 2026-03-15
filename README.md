# easyBot

`easyBot` is a security-focused local agent written in Go for macOS first, with a design that can expand to Linux and ACP later.

## What it does

- Chats through terminal or HTTP
- Uses an agent loop with tool calls
- Works with OpenAI-compatible chat completion APIs
- Manages and analyzes local files
- Runs guarded local commands without shell injection risk
- Keeps room for future ACP and non-ACP modes


## Security model

- All filesystem access is restricted to configured roots.
- Command execution does not invoke a shell.
- The agent is not allowed to delete files or stored data.
- Command execution is allowlist-first: known read-only commands are safe, approved state-changing commands require explicit human approval, and everything else is denied.
- Dangerous commands and interpreter-style execution are denied by policy.
- Commands that are allowed still run with a sanitized environment, bounded timeout, and bounded output capture.
- File writes require explicit approval.
- Each tool call is recorded in an audit log.
- The agent loop has step and timeout limits.

## Environment

Set these before running:

### api_key mode
api_key mode uses the OpenAI-compatible API path and an API key for auth. Set these env vars:

```bash
export EASYBOT_AUTH_MODE=api_key
export EASYBOT_MODEL=gpt-4.1-mini
export EASYBOT_BASE_URL=https://api.openai.com/v1
export EASYBOT_API_KEY=your_api_key
```

### codex_oauth mode
codex_oauth mode uses the Codex OAuth flow for auth and the Codex API path over HTTP/SSE. Set these env vars to use saved OAuth credentials from a previous login:

```bash
export EASYBOT_AUTH_MODE=codex_oauth
export EASYBOT_MODEL=gpt-5.4
```

sign in with Codex/ChatGPT OAuth interactively:

```bash
go run ./cmd/easybot login
```

That flow stores the OAuth bundle and runtime defaults in `~/.easybot/auth.json`. After login, `easyBot` will reuse the saved `auth_mode`, `base_url`, `model`, `access_token`, and `account_id` when the corresponding env vars are unset.

Optional:

```bash
export EASYBOT_ALLOWED_ROOTS=/Users/you,/tmp
export EASYBOT_MAX_STEPS=8
export EASYBOT_LOG_LEVEL=debug
export EASYBOT_APP_LOG=./easybot.log
export EASYBOT_AUDIT_LOG=/tmp/easybot-audit.jsonl
export EASYBOT_TOOL_OUTPUT_MAX_BYTES=65536
export EASYBOT_ACCESS_TOKEN=oauth_access_token
export EASYBOT_ACCOUNT_ID=chatgpt_account_id
```

## Run directly

Build the binary once:

```bash
make build
```

Then start terminal mode directly:

```bash
./easybot
```

You can also pass config as flags instead of exporting env vars:

```bash
./easybot --base-url http://localhost:11434/v1 --model your-model --roots /Users/you,/tmp
```

Or use the launcher script:

```bash
./run.sh
./run.sh http --listen :8080
./run.sh acp
```

## Run in terminal mode

```bash
go run ./cmd/easybot terminal
```

## Run HTTP mode

```bash
go run ./cmd/easybot http --listen :8080
```

Or with the built binary:

```bash
./easybot --http --listen :8080
```

## Run ACP mode

ACP runs over stdio:

```bash
./easybot acp
```

Or:

```bash
./run.sh acp
```

### HTTP endpoints

- `GET /healthz`
- `POST /v1/chat`

Example request:

```json
{
  "message": "Find large log files in my home directory",
  "approval": {
    "auto_approve_safe": true
  }
}
```

## Architecture

- `cmd/easybot`: CLI entrypoint
- `internal/agent`: agent loop and orchestration
- `internal/llm`: provider-agnostic chat client
- `internal/tools`: tool registry and local tools
- `internal/security`: path and command policy
- `internal/server`: HTTP API
- `internal/acp`: ACP stdio server and session handling

## ACP roadmap

Current code supports:

- `non-acp` runtime mode
- ACP stdio server with `initialize`, `session/new`, `session/list`, `session/prompt`, and `session/cancel`

What is intentionally deferred:

- richer ACP capabilities such as permission requests and persistent session loading
- MCP/skill integrations
- multi-agent orchestration

## Notes

- `EASYBOT_AUTH_MODE=api_key` uses the OpenAI-compatible `chat/completions` path and `EASYBOT_API_KEY`.
- `EASYBOT_AUTH_MODE=codex_oauth` uses the saved OAuth `access_token` plus `ChatGPT-Account-ID` against the Codex `/responses` path over HTTP/SSE.
- WebSocket/`auto` transport parity with Codex/OpenClaw is not implemented yet.
- `go run ./cmd/easybot login` uses the current OpenAI Codex OAuth browser flow with PKCE, a localhost callback on port `1455`, and a manual paste fallback for headless or remote sessions.
- `run.sh` no longer hardcodes provider credentials; saved config or env vars drive runtime auth.
- The terminal frontend supports interactive approval prompts.
- Application logs write to `easybot.log` by default instead of the active chat terminal. Set `EASYBOT_APP_LOG` or `--app-log` to move that file, or set `EASYBOT_LOG_LEVEL=off` to disable app logs entirely.
- The HTTP frontend only auto-approves safe read-only operations; file/data changes are denied unless you add a separate approval roundtrip.
- Built-in tools now include filename search and text-content search, so the agent can inspect files without falling back to shell as often.
- ACP currently keeps sessions in memory and auto-approves only safe read-only operations; writes and other data changes are denied until ACP approval callbacks are added.
