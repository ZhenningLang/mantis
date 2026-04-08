package main

import (
	"strings"
	"testing"
)

func TestRunForkRequiresSinglePrefix(t *testing.T) {
	err := runFork(nil)
	if err == nil || !strings.Contains(err.Error(), "usage: mantis fork <session-id-prefix>") {
		t.Fatalf("runFork() error = %v", err)
	}
}

func TestRunForkForksResolvedPrefix(t *testing.T) {
	originalResolve := resolveForkSessionID
	originalFork := forkSession
	t.Cleanup(func() {
		resolveForkSessionID = originalResolve
		forkSession = originalFork
	})

	resolveForkSessionID = func(prefix string) (string, error) {
		if prefix != "0290318a" {
			t.Fatalf("resolveForkSessionID() got %q, want %q", prefix, "0290318a")
		}
		return "0290318a-b368-4621-806d-8e2cf36bbf09", nil
	}

	var gotID string
	forkSession = func(id string) error {
		gotID = id
		return nil
	}

	if err := runFork([]string{"0290318a"}); err != nil {
		t.Fatalf("runFork() error = %v", err)
	}
	if gotID != "0290318a-b368-4621-806d-8e2cf36bbf09" {
		t.Fatalf("forkSession() got %q, want full session id", gotID)
	}
}
