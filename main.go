package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zhenninglang/mantis/internal/action"
	"github.com/zhenninglang/mantis/internal/completion"
	"github.com/zhenninglang/mantis/internal/compress"
	"github.com/zhenninglang/mantis/internal/config"
	"github.com/zhenninglang/mantis/internal/inspect"
	"github.com/zhenninglang/mantis/internal/session"
	"github.com/zhenninglang/mantis/internal/status"
	"github.com/zhenninglang/mantis/internal/summary"
	"github.com/zhenninglang/mantis/internal/tui"
)

var version = "dev"

var resolveForkSessionID = func(prefix string) (string, error) {
	sessions, err := session.LoadAll()
	if err != nil {
		return "", fmt.Errorf("load sessions: %w", err)
	}
	source, err := compress.ResolveSourceByPrefix(sessions, prefix)
	if err != nil {
		return "", err
	}
	return source.Meta.ID, nil
}

var forkSession = func(id string) error {
	droid, err := exec.LookPath("droid")
	if err != nil {
		return fmt.Errorf("droid not found: %w", err)
	}
	cmd := exec.Command(droid, "--fork", id)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "config":
			if err := config.RunSetup(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Printf("mantis %s\n", version)
			return
		case "status":
			if err := status.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "index":
			if err := runIndex(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "help", "-h", "--help":
			printHelp()
			return
		case "clean":
			if err := runClean(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "inspect":
			cfg := config.Load()
			if !cfg.HasLLM() {
				fmt.Fprintln(os.Stderr, "LLM not configured. Run `mantis config` first.")
				os.Exit(1)
			}
			if err := inspect.Run(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "compress":
			if err := runCompress(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "fork":
			if err := runFork(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "completion":
			if err := runCompletion(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun `mantis help` for usage.\n", os.Args[1])
			os.Exit(1)
		}
	}

	cfg := config.Load()

	sessions, err := session.LoadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading sessions: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found in ~/.factory/sessions/")
		os.Exit(0)
	}

	cwd, _ := os.Getwd()
	m := tui.New(sessions, version, cfg, cwd)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	model := result.(*tui.Model)
	if id := model.ResumeID(); id != "" {
		if err := resumeSession(id); err != nil {
			os.Exit(1)
		}
	}
}

func printHelp() {
	fmt.Printf(`mantis %s — Browse and search Droid chat sessions

Usage: mantis [command]

Commands:
  (none)     Launch interactive TUI (session viewer)
  inspect    Context Health Inspector — analyze sessions for optimization
  compress   Compress a session into a fresh handoff session and resume it
  fork       Fork a session by ID prefix and resume the fork
  completion Print shell completion script for bash/zsh/fish
  config     Configure LLM for smart search and inspect
  index      Generate AI summaries for all sessions (--force to regenerate all, --retry to redo empty ones)
  status     Show indexing status and statistics
  clean      Remove all empty sessions (no user messages)
  version    Print version
  help       Show this help

Keybindings (TUI):
  Type       Fuzzy search
  ↑/↓        Navigate
  Enter      Resume session
  Tab        Toggle project path
  Ctrl+P     Filter by project
  Ctrl+D     Delete session
  Ctrl+X     Batch delete
  Ctrl+R     Rename session
  Ctrl+S     Statistics panel
  Esc        Clear search / Clear filter / Quit
`, version)
}

func runIndex() error {
	cfg := config.Load()
	if !cfg.HasLLM() {
		return fmt.Errorf("LLM not configured. Run `mantis config` first")
	}

	flag := ""
	if len(os.Args) > 2 {
		flag = os.Args[2]
	}

	sessions, err := session.LoadAll()
	if err != nil {
		return err
	}

	switch flag {
	case "--force", "-f":
		os.RemoveAll(summary.Dir())
		fmt.Println("Cleared all existing summaries.")
	case "--retry", "-r":
		removed := summary.RemoveEmpty(sessions)
		fmt.Printf("Removed %d empty summaries for retry.\n", removed)
	}

	ch, total := summary.GenerateMissing(context.Background(), cfg.LLM, sessions)
	if total == 0 {
		fmt.Println("All sessions already indexed.")
		return nil
	}

	fmt.Printf("Indexing %d sessions...\n", total)
	errors := 0
	for p := range ch {
		if p.Err != nil {
			errors++
			fmt.Printf("  [%d/%d] ERROR %s: %v\n", p.Done, total, p.Current, p.Err)
		} else if p.Summary != nil && p.Summary.Title != "" {
			fmt.Printf("  [%d/%d] %s\n", p.Done, total, p.Summary.Title)
		} else {
			fmt.Printf("  [%d/%d] %s (skipped, no messages)\n", p.Done, total, p.Current)
		}
	}

	fmt.Printf("\nDone. Indexed %d/%d sessions", total-errors, total)
	if errors > 0 {
		fmt.Printf(" (%d errors)", errors)
	}
	fmt.Println()
	return nil
}

func runClean() error {
	sessions, err := session.LoadAll()
	if err != nil {
		return err
	}

	var empty []int
	for i := range sessions {
		hasUser := false
		for _, msg := range sessions[i].Messages {
			if msg.Role == "user" {
				hasUser = true
				break
			}
		}
		if !hasUser {
			empty = append(empty, i)
		}
	}

	if len(empty) == 0 {
		fmt.Println("No empty sessions found.")
		return nil
	}

	fmt.Printf("Found %d empty sessions (no user messages). Delete all? [y/N] ", len(empty))
	var answer string
	fmt.Scanln(&answer)
	if answer != "y" && answer != "Y" {
		fmt.Println("Cancelled.")
		return nil
	}

	deleted := 0
	for _, idx := range empty {
		if err := action.Delete(&sessions[idx]); err != nil {
			fmt.Printf("  Failed to delete %s: %v\n", sessions[idx].Meta.ID, err)
		} else {
			deleted++
		}
	}
	fmt.Printf("Deleted %d empty sessions.\n", deleted)
	return nil
}

func runCompress(args []string) error {
	id, err := compress.Run(args)
	if err != nil {
		return err
	}
	fmt.Printf("[compress] Resuming new session %s...\n", id)
	if err := resumeSession(id); err != nil {
		return fmt.Errorf("compressed session %s created, but resume failed: %w", id, err)
	}
	return nil
}

func runFork(args []string) error {
	if len(args) != 1 || args[0] == "" {
		return fmt.Errorf("usage: mantis fork <session-id-prefix>")
	}
	id, err := resolveForkSessionID(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("[fork] Forking session %s...\n", id)
	return forkSession(id)
}

func runCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: mantis completion <bash|zsh|fish>")
	}
	script, err := completion.Generate(args[0])
	if err != nil {
		return err
	}
	fmt.Print(script)
	return nil
}

func resumeSession(id string) error {
	droid, err := exec.LookPath("droid")
	if err != nil {
		return fmt.Errorf("droid not found: %w", err)
	}
	cmd := exec.Command(droid, "-r", id)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
