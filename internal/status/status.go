package status

import (
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zhenninglang/mantis/internal/config"
	"github.com/zhenninglang/mantis/internal/session"
	"github.com/zhenninglang/mantis/internal/summary"
)

func Run() error {
	cfg := config.Load()
	sessions, err := session.LoadAll()
	if err != nil {
		return err
	}

	// count projects and summaries
	projects := map[string]int{}
	indexed := map[string]int{}
	totalIndexed := 0

	for i := range sessions {
		p := sessions[i].ProjectShort()
		projects[p]++
		if summary.HasSummary(sessions[i].FilePath) {
			indexed[p]++
			totalIndexed++
		}
	}

	sep := strings.Repeat("─", 45)
	fmt.Println(sep)

	fmt.Printf("Sessions:    %d total, %d projects\n", len(sessions), len(projects))

	pct := float64(0)
	if len(sessions) > 0 {
		pct = float64(totalIndexed) / float64(len(sessions)) * 100
	}
	fmt.Printf("Summaries:   %d/%d (%.1f%%) indexed\n", totalIndexed, len(sessions), pct)

	if cfg.HasLLM() {
		host := cfg.LLM.BaseURL
		if u, err := url.Parse(cfg.LLM.BaseURL); err == nil {
			host = u.Host
		}
		fmt.Printf("LLM Config:  %s · %s\n", host, cfg.LLM.Model)
	} else {
		fmt.Println("LLM Config:  Not configured. Run `mantis config`")
	}

	fmt.Printf("Storage:     %s (%s)\n", summary.Dir(), dirSize(summary.Dir()))

	fmt.Println(sep)
	fmt.Println("Top Projects:")

	type kv struct {
		name    string
		total   int
		indexed int
	}
	var sorted []kv
	for k, v := range projects {
		sorted = append(sorted, kv{k, v, indexed[k]})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].total > sorted[j].total })

	limit := 10
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for _, p := range sorted[:limit] {
		fmt.Printf("  %-20s %3d sessions (%d indexed)\n", p.name, p.total, p.indexed)
	}
	if len(sorted) > limit {
		fmt.Printf("  ... and %d more\n", len(sorted)-limit)
	}

	fmt.Println(sep)
	return nil
}

func dirSize(path string) string {
	var size int64
	filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			size += info.Size()
		}
		return nil
	})

	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
}
