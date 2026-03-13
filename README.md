# mantis

Interactive TUI for browsing, searching, and managing [Droid](https://docs.factory.ai) chat sessions.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss).

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
mantis
```

## Keybindings

| Key | Action |
|-----|--------|
| Type | Fuzzy search (title, project, content) |
| `↑` / `↓` | Navigate |
| `Enter` | Resume session (`droid -r`) |
| `Tab` | Toggle project short name / full path |
| `Ctrl+D` | Delete session |
| `Ctrl+X` | Batch delete (Tab to mark, d to confirm) |
| `Ctrl+R` | Rename session |
| `Ctrl+S` | Statistics panel |
| `Esc` | Clear search / Quit |

## Data

Reads from `~/.factory/sessions/`. Each session is a `.jsonl` (conversation) + `.settings.json` (metadata) pair.

## License

[MIT](LICENSE)
