package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type LLMConfig struct {
	BaseURL string `yaml:"base_url"`
	APIKey  string `yaml:"api_key"`
	Model   string `yaml:"model"`
}

type Config struct {
	LLM LLMConfig `yaml:"llm"`
}

func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mantis")
}

func path() string {
	return filepath.Join(Dir(), "config.yaml")
}

func Load() Config {
	data, err := os.ReadFile(path())
	if err != nil {
		return Config{}
	}
	var cfg Config
	yaml.Unmarshal(data, &cfg)
	return cfg
}

func (c Config) HasLLM() bool {
	return c.LLM.APIKey != "" && c.LLM.Model != ""
}

func save(cfg Config) error {
	if err := os.MkdirAll(Dir(), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path(), data, 0600)
}

func RunSetup() error {
	cfg := Load()
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("mantis - LLM Configuration")
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println()

	cfg.LLM.BaseURL = prompt(reader, "Base URL", cfg.LLM.BaseURL, "https://api.openai.com/v1")
	cfg.LLM.APIKey = prompt(reader, "API Key", cfg.LLM.APIKey, "")
	cfg.LLM.Model = prompt(reader, "Model", cfg.LLM.Model, "gpt-4o-mini")

	if err := save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("\nConfig saved to %s\n", path())
	return nil
}

func prompt(reader *bufio.Reader, label, current, fallback string) string {
	hint := fallback
	if current != "" {
		hint = current
	}
	if hint != "" {
		fmt.Printf("  %s [%s]: ", label, hint)
	} else {
		fmt.Printf("  %s: ", label)
	}

	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		if current != "" {
			return current
		}
		return fallback
	}
	return line
}
