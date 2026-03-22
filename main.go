package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zhenninglang/mantis/internal/action"
	"github.com/zhenninglang/mantis/internal/config"
	"github.com/zhenninglang/mantis/internal/session"
	"github.com/zhenninglang/mantis/internal/status"
	"github.com/zhenninglang/mantis/internal/summary"
	"github.com/zhenninglang/mantis/internal/tui"
)

var version = "dev"

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
		droid, err := exec.LookPath("droid")
		if err != nil {
			fmt.Fprintf(os.Stderr, "droid not found: %v\n", err)
			os.Exit(1)
		}
		cmd := exec.Command(droid, "-r", id)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
	}
}

func printHelp() {
	fmt.Printf(`mantis %s — Browse and search Droid chat sessions

Usage: mantis [command]

Commands:
  (none)     Launch interactive TUI
  config     Configure LLM for smart search
  index      Generate AI summaries for all sessions (--force to regenerate all)
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

	force := len(os.Args) > 2 && (os.Args[2] == "--force" || os.Args[2] == "-f")

	sessions, err := session.LoadAll()
	if err != nil {
		return err
	}

	if force {
		// clear all existing summaries
		os.RemoveAll(summary.Dir())
		fmt.Println("Cleared all existing summaries.")
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
