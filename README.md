# Droid Toolkits

<img src="icon.svg" width="80" align="right" alt="Droid Toolkits">

Interactive TUI for browsing, searching, and managing [Droid](https://docs.factory.ai) chat sessions.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss).

![screenshot](screenshot.png)

## Features

- **Fuzzy search** across session titles, project names, user messages, and AI-generated summaries
- **Project auto-filter** — automatically scopes to sessions from the current directory
- **AI-powered indexing** — generates summaries and keywords for each session via any OpenAI-compatible LLM
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
mantis config           # Configure LLM for smart search
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

Configure an LLM to auto-generate summaries and keywords for each session:

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

## Data

| Path | Content |
|------|---------|
| `~/.factory/sessions/` | Session files (`.jsonl` + `.settings.json`) |
| `~/.mantis/config.yaml` | LLM configuration |
| `~/.mantis/summaries/` | Cached AI summaries |

## License

[MIT](LICENSE)
