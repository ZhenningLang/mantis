package compress

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhenninglang/mantis/internal/session"
)

func Run(args []string) (string, error) {
	return defaultRunner().run(args)
}

type runner struct {
	out                     io.Writer
	loadSessions            func() ([]session.Session, error)
	parseEvents             func(path string) ([]session.RawEvent, error)
	loadDroidSettings       func(path string) (DroidSettings, error)
	generateHandoff         func(auth LLMAuth, model string, input CompressionInput) (*LLMHandoff, error)
	createCompressedSession func(source session.Session, handoff LLMHandoff) (string, error)
	handoffProgressInterval time.Duration
}

func defaultRunner() runner {
	return runner{
		out:                     os.Stdout,
		loadSessions:            session.LoadAll,
		parseEvents:             session.ParseAllEvents,
		loadDroidSettings:       LoadDroidSettings,
		generateHandoff:         GenerateHandoff,
		createCompressedSession: CreateCompressedSession,
		handoffProgressInterval: 5 * time.Second,
	}
}

func (r runner) run(args []string) (string, error) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return "", fmt.Errorf("usage: mantis compress <session-id-prefix>")
	}
	prefix := strings.TrimSpace(args[0])

	r.logf("Resolving source session for prefix %q...", prefix)
	all, err := r.loadSessions()
	if err != nil {
		return "", fmt.Errorf("load sessions: %w", err)
	}

	source, err := ResolveSourceByPrefix(all, prefix)
	if err != nil {
		return "", err
	}
	r.logf("Matched source session %s (%s).", source.Meta.ID, displayTitle(source.Meta.Title))

	r.logf("Parsing source session events...")
	events, err := r.parseEvents(source.FilePath)
	if err != nil {
		return "", fmt.Errorf("parse source session: %w", err)
	}
	r.logf("Parsed %d events from %s.", len(events), source.FilePath)

	input := BuildCompressionInput(source, events)
	if !input.HasVisibleContent() {
		return "", fmt.Errorf("source session has no visible content to compress")
	}
	r.logf(
		"Compaction window prepared: %d summarized turns, %d preserved turns, %d active skills.",
		input.Compaction.RemovedCount,
		len(input.RecentWindow),
		len(input.Compaction.ActiveSkills),
	)

	r.logf("Loading Droid settings...")
	settings, err := r.loadDroidSettings(defaultDroidSettingsPath())
	if err != nil {
		return "", err
	}

	model, err := ResolveCompressionModel(source.Settings.Model, settings)
	if err != nil {
		return "", err
	}
	r.logf("Resolved compression model %q from source model %q.", model, source.Settings.Model)
	auth, err := ResolveCompressionAuth(settings)
	if err != nil {
		return "", err
	}
	r.logf("Resolved compression auth (baseURL=%s).", auth.BaseURL)

	r.logf("Generating structured handoff via LLM...")
	progress := r.startHandoffProgressLogs()
	handoff, err := r.generateHandoff(auth, model, input)
	progress.Stop()
	if err != nil {
		return "", err
	}
	handoff.RecentTranscript = buildRecentTranscript(input.RecentWindow)
	r.logf("Structured handoff generated in %s.", formatCompressDuration(time.Since(progress.startedAt)))

	r.logf("Writing compressed session...")
	newID, err := r.createCompressedSession(source, *handoff)
	if err != nil {
		return "", err
	}
	r.logf("Created compressed session %s.", newID)
	return newID, nil
}

func ResolveSourceByPrefix(sessions []session.Session, prefix string) (session.Session, error) {
	var matches []session.Session
	for _, s := range sessions {
		if strings.HasPrefix(s.Meta.ID, prefix) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return session.Session{}, fmt.Errorf("no session matches prefix %q", prefix)
	case 1:
		return matches[0], nil
	default:
		var ids []string
		for _, s := range matches {
			ids = append(ids, s.Meta.ID)
		}
		return session.Session{}, fmt.Errorf("multiple sessions match prefix %q: %s", prefix, strings.Join(ids, ", "))
	}
}

func LoadDroidSettings(path string) (DroidSettings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DroidSettings{}, fmt.Errorf("read droid settings: %w", err)
	}

	var settings DroidSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return DroidSettings{}, fmt.Errorf("parse droid settings: %w", err)
	}
	return settings, nil
}

func ResolveCompressionModel(sourceModel string, settings DroidSettings) (string, error) {
	sourceModel = strings.TrimSpace(sourceModel)
	if sourceModel == "" {
		return "", fmt.Errorf("source session model is empty")
	}
	if !strings.HasPrefix(sourceModel, "custom:") {
		return sourceModel, nil
	}

	model, ok := findCustomModel(settings.CustomModels, sourceModel)
	if !ok || strings.TrimSpace(model.Model) == "" {
		return "", fmt.Errorf("source custom model %q not found in droid settings", sourceModel)
	}
	return model.Model, nil
}

func ResolveCompressionAuth(settings DroidSettings) (LLMAuth, error) {
	defaultID := strings.TrimSpace(settings.SessionDefaultSettings.Model)
	if defaultID == "" {
		return LLMAuth{}, fmt.Errorf("default custom model is not configured in droid settings")
	}
	model, ok := findCustomModel(settings.CustomModels, defaultID)
	if !ok {
		return LLMAuth{}, fmt.Errorf("default custom model %q not found in droid settings", defaultID)
	}
	if strings.TrimSpace(model.BaseURL) == "" || strings.TrimSpace(model.APIKey) == "" {
		return LLMAuth{}, fmt.Errorf("default custom model %q is missing baseUrl or apiKey", defaultID)
	}

	return LLMAuth{
		BaseURL:   model.BaseURL,
		APIKey:    model.APIKey,
		Provider:  model.Provider,
		ExtraArgs: model.ExtraArgs,
	}, nil
}

func findCustomModel(models []CustomModel, id string) (CustomModel, bool) {
	for _, model := range models {
		if model.ID == id {
			return model, true
		}
	}
	return CustomModel{}, false
}

func defaultDroidSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".factory", "settings.json")
}

func (input CompressionInput) HasVisibleContent() bool {
	return input.Anchors.InitialUserGoal != "" || input.Anchors.LastUserRequest != "" || len(input.RecentWindow) > 0
}

func (r runner) logf(format string, args ...any) {
	if r.out == nil {
		return
	}
	fmt.Fprintf(r.out, "[compress] "+format+"\n", args...)
}

func (r runner) warnf(format string, args ...any) {
	r.logf(format, args...)
}

func displayTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Untitled"
	}
	return title
}

type stopProgressFunc struct {
	stop      chan struct{}
	done      chan struct{}
	startedAt time.Time
}

func (f stopProgressFunc) Stop() {
	close(f.stop)
	<-f.done
}

func (r runner) startHandoffProgressLogs() stopProgressFunc {
	interval := r.handoffProgressInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}

	handle := stopProgressFunc{
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
		startedAt: time.Now(),
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(handle.done)

		for {
			select {
			case <-ticker.C:
				r.logf("Still generating structured handoff... elapsed %s.", formatCompressDuration(time.Since(handle.startedAt)))
			case <-handle.stop:
				return
			}
		}
	}()

	return handle
}

func formatCompressDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	return d.Round(time.Second).String()
}
