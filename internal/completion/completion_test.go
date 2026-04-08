package completion

import (
	"strings"
	"testing"
)

func TestGenerateIncludesInspectAndCompressForAllShells(t *testing.T) {
	shells := []string{"bash", "zsh", "fish"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			script, err := Generate(shell)
			if err != nil {
				t.Fatalf("Generate(%q) error = %v", shell, err)
			}
			for _, command := range []string{"inspect", "compress", "fork"} {
				if !strings.Contains(script, command) {
					t.Fatalf("Generate(%q) missing command %q in script: %s", shell, command, script)
				}
			}
		})
	}
}

func TestGenerateRejectsUnknownShell(t *testing.T) {
	_, err := Generate("powershell")
	if err == nil || !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("Generate() error = %v, want unsupported shell", err)
	}
}
