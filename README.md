# easyBot

`easyBot` is a security-focused local agent written in Go for macOS first, with a design that can expand to Linux and ACP later.

## What it does

- Chats through terminal or HTTP
- Uses an agent loop with tool calls
- Works with OpenAI-compatible chat completion APIs
- Manages and analyzes local files
- Runs guarded local commands without shell injection risk
- Keeps room for future ACP and non-ACP modes

## Why the first HTTP server uses the Go standard library

I chose `net/http` for the initial implementation:

- smallest dependency surface
- easier to audit
- enough for a JSON API right now
- simple to replace with `chi` or `gin` later if you want richer middleware

If you want, I can switch the HTTP layer to `chi` next.

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

```bash
export EASYBOT_MODEL=gpt-4.1-mini
export EASYBOT_BASE_URL=https://api.openai.com/v1
export EASYBOT_AUTH_MODE=api_key
```

If you are using API-key mode:

```bash
export EASYBOT_API_KEY=your_api_key
```

Or sign in with Codex/ChatGPT OAuth interactively:

```bash
go run ./cmd/easybot login
```

That flow stores the OAuth bundle and runtime defaults in `~/.easybot/auth.json`. After login, `easyBot` will reuse the saved `auth_mode`, `base_url`, `model`, `access_token`, and `account_id` when the corresponding env vars are unset.

Optional:

```bash
export EASYBOT_ALLOWED_ROOTS=/Users/you,/tmp
export EASYBOT_MAX_STEPS=8
export EASYBOT_LOG_LEVEL=debug
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
- `EASYBOT_AUTH_MODE=codex_oauth` uses the saved OAuth `access_token` plus `ChatGPT-Account-ID` against the Codex `/responses` path.
- `go run ./cmd/easybot login` uses the current OpenAI Codex OAuth browser flow with PKCE, a localhost callback on port `1455`, and a manual paste fallback for headless or remote sessions.
- `run.sh` no longer hardcodes provider credentials; saved config or env vars drive runtime auth.
- The terminal frontend supports interactive approval prompts.
- The HTTP frontend only auto-approves safe read-only operations; file/data changes are denied unless you add a separate approval roundtrip.
- Built-in tools now include filename search and text-content search, so the agent can inspect files without falling back to shell as often.
- ACP currently keeps sessions in memory and auto-approves only safe read-only operations; writes and other data changes are denied until ACP approval callbacks are added.
