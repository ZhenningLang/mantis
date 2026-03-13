# mantis

Interactive TUI for browsing, searching, and managing [Droid](https://docs.factory.ai) chat sessions.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss).

![screenshot](screenshot.png)

## Install

```bash
go install github.com/zhenninglang/mantis@latest
```

Or build from source:

```bash
git clone https://github.com/zhenninglang/mantis.git
cd mantis
go build -o mantis .
```

## Usage

```bash
mantis              # Launch TUI
mantis config       # Configure LLM for smart search
mantis status       # Show indexing status and statistics
mantis version      # Print version
```

## Keybindings

| Key | Action |
|-----|--------|
| Type | Fuzzy search (title, project, content) |
| `↑` / `↓` | Navigate |
| `Enter` | Resume session (`droid -r`) |
| `Tab` | Toggle project short name / full path |
| `Ctrl+P` | Filter by project |
| `Ctrl+D` | Delete session |
| `Ctrl+X` | Batch delete (Tab to mark, d to confirm) |
| `Ctrl+R` | Rename session |
| `Ctrl+S` | Statistics panel |
| `Esc` | Clear search / Clear project filter / Quit |

## Smart Search

By default, mantis searches across session titles, project names, and user messages using fuzzy matching.

For better search results, configure an LLM to auto-generate summaries and keywords for each session:

```bash
mantis config
```

This will prompt you for:
- **Base URL** — OpenAI-compatible API endpoint (default: `https://api.openai.com/v1`)
- **API Key** — Your API key
- **Model** — Model name (default: `gpt-4o-mini`)

Any OpenAI-compatible provider works (OpenAI, Deepseek, Ollama, etc.).

Once configured, mantis will automatically generate summaries for new sessions in the background on startup. The indexing progress is shown in the header (`Indexing: 42/128`). Summaries are cached in `~/.mantis/summaries/` so they only need to be generated once per session.

## Data

- Sessions: `~/.factory/sessions/` — `.jsonl` (conversation) + `.settings.json` (metadata)
- Config: `~/.mantis/config.yaml`
- Summaries: `~/.mantis/summaries/`

## License

[MIT](LICENSE)
