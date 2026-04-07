package inspect

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zhenninglang/mantis/internal/config"
	"github.com/zhenninglang/mantis/internal/session"
)

// Run is the main entry point for `mantis inspect`.
func Run(cfg config.Config) error {
	fmt.Println()
	fmt.Printf("%s[1/4]%s 扫描 sessions...\n", colorBold, colorReset)

	all, err := session.LoadAll()
	if err != nil {
		return fmt.Errorf("load sessions: %w", err)
	}
	fmt.Printf("  找到 %d 个 session\n", len(all))

	picked := SelectSessions(all, 3)
	if len(picked) == 0 {
		return fmt.Errorf("没有找到足够长的 session（需要 >= %d 条消息）", minMessages)
	}

	fmt.Printf("  筛选出 %d 个最适合分析的 session：\n", len(picked))
	for _, s := range picked {
		sid := s.Meta.ID
		if len(sid) > 8 {
			sid = sid[:8]
		}
		fmt.Printf("    • %s (%s, %d msg, %s)\n", sid, s.Project, len(s.Messages), s.Settings.Model)
	}
	fmt.Println()

	// static analysis
	fmt.Printf("%s[2/4]%s 静态分析...\n", colorBold, colorReset)
	var analyses []SessionAnalysis
	for _, s := range picked {
		a := Analyze(s)
		analyses = append(analyses, a)
		sid := s.Meta.ID
		if len(sid) > 8 {
			sid = sid[:8]
		}
		fmt.Printf("  ✓ %s: %d msg, tool_result %.1f%%, cache %.0f%%\n",
			sid, a.MessageCount, a.Distribution.Pct(a.Distribution.ToolResult), a.CacheAnalysis.HitRate)
	}
	fmt.Println()

	// agent analysis
	fmt.Printf("%s[3/4]%s Agent 分析 (模型: %s)...\n", colorBold, colorReset, cfg.LLM.Model)
	agentResult, err := RunAgentAnalysis(context.Background(), cfg.LLM, analyses)
	if err != nil {
		fmt.Printf("  %s⚠ Agent 分析失败: %v%s\n", colorYellow, err, colorReset)
		fmt.Println("  将只输出静态分析结果。")
		agentResult = ""
	} else {
		fmt.Println("  ✓ 分析完成")
	}
	fmt.Println()

	// report
	fmt.Printf("%s[4/4]%s 生成报告...\n\n", colorBold, colorReset)

	report := InspectReport{
		Sessions:      analyses,
		AgentAnalysis: agentResult,
	}

	output := PrintReport(report)
	fmt.Print(output)

	// save to file
	if err := saveReport(output); err != nil {
		fmt.Printf("%s  ⚠ 报告保存失败: %v%s\n", colorYellow, err, colorReset)
	}

	return nil
}

func saveReport(content string) error {
	dir := filepath.Join(config.Dir(), "reports")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	filename := fmt.Sprintf("inspect-%s.txt", time.Now().Format("2006-01-02T15-04-05"))
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("  报告已保存: %s\n", path)
	return nil
}
