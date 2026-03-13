package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func sessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".factory", "sessions")
}

func LoadAll() ([]Session, error) {
	root := sessionsDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("cannot read sessions dir: %w", err)
	}

	var sessions []Session

	for _, entry := range entries {
		if entry.IsDir() {
			project := dirToProject(entry.Name())
			projectFull := dirToPath(entry.Name())
			dirPath := filepath.Join(root, entry.Name())
			ss, _ := loadFromDir(dirPath, project, projectFull)
			sessions = append(sessions, ss...)
		} else if strings.HasSuffix(entry.Name(), ".jsonl") {
			id := strings.TrimSuffix(entry.Name(), ".jsonl")
			s, err := loadSession(root, id, "", "")
			if err == nil {
				sessions = append(sessions, s)
			}
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	return sessions, nil
}

func loadFromDir(dir, project, projectFull string) ([]Session, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			id := strings.TrimSuffix(e.Name(), ".jsonl")
			s, err := loadSession(dir, id, project, projectFull)
			if err == nil {
				sessions = append(sessions, s)
			}
		}
	}
	return sessions, nil
}

func loadSession(dir, id, project, projectFull string) (Session, error) {
	jsonlPath := filepath.Join(dir, id+".jsonl")
	settingsPath := filepath.Join(dir, id+".settings.json")

	info, err := os.Stat(jsonlPath)
	if err != nil {
		return Session{}, err
	}

	s := Session{
		Project:     project,
		ProjectFull: projectFull,
		ModTime:     info.ModTime(),
		FilePath:    jsonlPath,
	}

	// parse metadata from first line
	meta, msgs, err := parseJSONL(jsonlPath)
	if err == nil {
		s.Meta = meta
		s.Messages = msgs
	}
	if s.Meta.Title == "" {
		s.Meta.Title = "Untitled"
	}
	if s.Meta.ID == "" {
		s.Meta.ID = id
	}

	// parse settings
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &s.Settings)
	}

	// prefer cwd from metadata as source of truth
	if s.Meta.WorkingDirectory != "" {
		s.ProjectFull = s.Meta.WorkingDirectory
		s.Project = filepath.Base(s.Meta.WorkingDirectory)
	}

	return s, nil
}

func parseJSONL(path string) (SessionMeta, []Message, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionMeta{}, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	var meta SessionMeta
	var messages []Message
	lineNum := 0
	msgCount := 0
	maxMessages := 20

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if lineNum == 0 {
			json.Unmarshal(line, &meta)
		} else if msgCount < maxMessages {
			var wrapper struct {
				Type    string  `json:"type"`
				Message Message `json:"message"`
			}
			if json.Unmarshal(line, &wrapper) == nil && wrapper.Type == "message" && wrapper.Message.Role != "" {
				messages = append(messages, wrapper.Message)
				msgCount++
			}
		} else {
			break
		}
		lineNum++
	}

	return meta, messages, nil
}

func dirToProject(dirName string) string {
	p := dirToPath(dirName)
	if p != "" {
		if base := filepath.Base(p); base != "." && base != "/" {
			return base
		}
	}
	return dirName
}

func dirToPath(dirName string) string {
	if dirName == "" {
		return ""
	}

	parts := strings.Split(dirName, "-")
	var segments []string
	for _, p := range parts {
		if p != "" {
			segments = append(segments, p)
		}
	}
	if len(segments) == 0 {
		return ""
	}

	// probe filesystem to resolve ambiguous `-` separators
	if resolved := probePath(segments); resolved != "" {
		return resolved
	}

	// fallback: naive replacement (path may not exist on this machine)
	return "/" + strings.TrimLeft(strings.ReplaceAll(dirName, "-", "/"), "/")
}

// probePath tries to reconstruct a real filesystem path from segments
// that were split by `-`. At each level, it tries the single segment first;
// if no directory matches, it greedily combines with subsequent segments.
func probePath(segments []string) string {
	current := "/"
	for i := 0; i < len(segments); {
		found := false
		candidate := ""
		for j := i; j < len(segments); j++ {
			if j == i {
				candidate = segments[j]
			} else {
				candidate += "-" + segments[j]
			}
			test := filepath.Join(current, candidate)
			if info, err := os.Stat(test); err == nil && info.IsDir() {
				current = test
				i = j + 1
				found = true
				break
			}
		}
		if !found {
			return ""
		}
	}
	return current
}
