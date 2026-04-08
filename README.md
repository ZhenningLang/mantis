# Droid Toolkits

<img src="icon.svg" width="80" align="right" alt="Droid Toolkits">

Interactive TUI and CLI for browsing, searching, managing, and inspecting [Droid](https://docs.factory.ai) chat sessions.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss).

![screenshot](screenshot.png)

## Features

- **Fuzzy search** across session titles, project names, user messages, and AI-generated summaries
- **Project auto-filter** — automatically scopes to sessions from the current directory
- **AI-powered indexing** — generates summaries and keywords for each session via any OpenAI-compatible LLM
- **Context Health Inspector** — analyzes representative sessions for prompt bloat, tool overhead, and cache efficiency
- **Session compression handoff** — condenses a long session into a structured resume-ready handoff, appends a deterministic recent transcript tail, and jumps into the fresh session
- **Session fork by prefix** — resolves a session ID prefix and delegates to `droid --fork`
- **Preview panel** — metadata, AI topics, and head/tail conversation turns
- **Session management** — resume, rename, delete, batch delete
- **Cross-process safe** — file-level locking prevents duplicate indexing

## Install

```bash
go install github.com/zhenninglang/mantis@latest
```

Or build from source:

```bash
git clone https://github.com/ZhenningLang/Droid-Toolkits.git
cd Droid-Toolkits
go build -o mantis .
```

## Usage

```bash
mantis                  # Launch TUI (auto-filters to current project)
mantis inspect          # Analyze representative sessions for context optimization
mantis compress <id>    # Create a structured compressed handoff session, then droid -r into it
mantis fork <id>        # Resolve a session ID prefix and droid --fork it
mantis completion zsh   # Print shell completion script
mantis config           # Configure LLM for smart search and inspect
mantis index            # Generate AI summaries for all sessions
mantis index --retry    # Re-index only sessions with empty summaries
mantis index --force    # Regenerate all summaries from scratch
mantis status           # Show indexing status and statistics
mantis clean            # Remove all empty sessions (no user messages)
mantis version          # Print version
mantis help             # Show help
```

## Keybindings

| Key | Action |
|-----|--------|
| Type | Fuzzy search (title, project, content, AI summary) |
| `↑` / `↓` | Navigate |
| `Enter` | Resume session (`droid -r`) |
| `Tab` | Toggle project short name / full path |
| `Ctrl+P` | Filter by project (or switch to all) |
| `Ctrl+D` | Delete session |
| `Ctrl+X` | Batch delete (Tab to mark, d to confirm) |
| `Ctrl+R` | Rename session |
| `Ctrl+S` | Statistics panel |
| `Esc` | Clear search → Clear project filter → Quit |

## Smart Search

Configure an LLM to enable both smart search and inspect analysis:

```bash
mantis config
```

This will prompt you for:
- **Base URL** — OpenAI-compatible API endpoint (default: `https://api.openai.com/v1`)
- **API Key** — Your API key
- **Model** — Model name (default: `gpt-4o-mini`)

Any OpenAI-compatible provider works (OpenAI, Deepseek, Ollama, etc.).

Once configured, mantis indexes sessions in the background on startup. The status bar shows progress: `995 total, 271 shown, 537 indexed, 231 skipped, 227 waiting`.

Summaries are cached in `~/.mantis/summaries/` and only generated once per session. Multiple mantis instances can run concurrently without duplicate indexing.

## Context Health Inspector

Run:

```bash
mantis inspect
```

`mantis inspect` scans local sessions, selects a few representative long-running sessions, and reports:

- context distribution across `system_prompt`, `system_reminder`, `thinking`, `tool_use`, `tool_result`, and user/assistant text
- tool hotspots by call count and returned content size
- token usage and cache hit rate
- an LLM-generated diagnosis with optimization suggestions

LLM-backed features now default to streaming OpenAI-compatible requests, which keeps `inspect`, `index`, and `compress` compatible with local CLI proxy endpoints that emit text through SSE deltas instead of the final non-streaming JSON body.

Reports are saved locally for later review.

## Session Compression

Run:

```bash
mantis compress <session-id-prefix>
```

`mantis compress` resolves the source session by ID prefix, prints stage-by-stage progress to the terminal, then runs an anchor-based compaction pass: it skips content before the latest prior compressed handoff, preserves a token-budgeted recent window verbatim for the LLM, keeps active skill names, groups older turns into compacted history phases, filters Droid-internal `BYOK Error:` noise out of the preserved transcript, appends a deterministic recent transcript tail built from the latest visible turns with its own cap, calls the streaming Chat Completions API through the shared SSE client, emits periodic "still generating" heartbeat logs while the LLM handoff is in flight, writes a fresh session beside the original files, and immediately resumes it with `droid -r`. If the compression API returns empty or invalid output, the command still fails fast instead of silently degrading to a fallback handoff.

## Session Fork

Run:

```bash
mantis fork <session-id-prefix>
```

`mantis fork` resolves the prefix to a unique local session ID and then shells out to `droid --fork <full-id>` so you can branch from an earlier conversation without looking up the full UUID manually.

## Shell Completion

Run one of:

```bash
mantis completion bash
mantis completion zsh
mantis completion fish
```

The generated script includes `inspect`, `compress`, and `fork` in the command completion list.

## Data

| Path | Content |
|------|---------|
| `~/.factory/sessions/` | Session files (`.jsonl` + `.settings.json`) |
| `~/.mantis/config.yaml` | LLM configuration |
| `~/.mantis/summaries/` | Cached AI summaries |
| `~/.mantis/reports/` | Saved inspect reports |

## License

[MIT](LICENSE)
