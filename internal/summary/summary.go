package summary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhenninglang/mantis/internal/config"
)

type Topic struct {
	Summary  string   `json:"summary"`
	Keywords []string `json:"keywords"`
}

type Summary struct {
	Title       string    `json:"title"`
	Topics      []Topic   `json:"topics"`
	GeneratedAt time.Time `json:"generated_at"`
	Model       string    `json:"model"`
}

func Dir() string {
	return filepath.Join(config.Dir(), "summaries")
}

func SummaryPath(sessionFilePath string) string {
	// sessionFilePath: ~/.factory/sessions/{project_dir}/{id}.jsonl
	// summaryPath:     ~/.mantis/summaries/{project_dir}/{id}.summary.json
	dir := filepath.Dir(sessionFilePath)
	projectDir := filepath.Base(dir)

	id := strings.TrimSuffix(filepath.Base(sessionFilePath), ".jsonl")
	return filepath.Join(Dir(), projectDir, id+".summary.json")
}

func LoadSummary(sessionFilePath string) *Summary {
	data, err := os.ReadFile(SummaryPath(sessionFilePath))
	if err != nil {
		return nil
	}
	var s Summary
	if json.Unmarshal(data, &s) != nil {
		return nil
	}
	return &s
}

func SaveSummary(sessionFilePath string, s *Summary) error {
	p := SummaryPath(sessionFilePath)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

func HasSummary(sessionFilePath string) bool {
	_, err := os.Stat(SummaryPath(sessionFilePath))
	return err == nil
}

// SearchText returns a concatenated string of all summary fields for fuzzy search.
func (s *Summary) SearchText() string {
	var parts []string
	parts = append(parts, s.Title)
	for _, t := range s.Topics {
		parts = append(parts, t.Summary)
		parts = append(parts, t.Keywords...)
	}
	return strings.Join(parts, " ")
}
