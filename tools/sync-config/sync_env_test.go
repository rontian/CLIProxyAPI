package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncEnvAddsMissingKeysOnly(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, ".env.example")
	target := filepath.Join(dir, ".env")
	if err := os.WriteFile(example, []byte("A=1\n\n# B comment\nB=2\n# C=3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("A=local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := syncEnv(example, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.changed() {
		t.Fatal("changed = false, want true")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "A=local\n") || strings.Contains(got, "A=1\n") {
		t.Fatalf("existing env value was not preserved: %q", got)
	}
	if !strings.Contains(got, "B=2\n") || !strings.Contains(got, "# C=3\n") {
		t.Fatalf("missing env additions: %q", got)
	}
	if len(report.addedKeys) != 2 {
		t.Fatalf("expected 2 added keys, got %d: %v", len(report.addedKeys), report.addedKeys)
	}
	if len(report.commentedKeys) != 0 {
		t.Fatalf("expected 0 commented keys, got %d: %v", len(report.commentedKeys), report.commentedKeys)
	}
}

func TestSyncEnvCommentsOutRemovedKeys(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, ".env.example")
	target := filepath.Join(dir, ".env")
	// Example has A and B only.
	if err := os.WriteFile(example, []byte("A=1\nB=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Target has A (in example), C (not in example, should be commented),
	// and #D (already commented, should be left alone).
	if err := os.WriteFile(target, []byte("A=local\nC=old\n#D=already_commented\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := syncEnv(example, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if !report.changed() {
		t.Fatal("changed = false, want true")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	// A should be preserved.
	if !strings.Contains(got, "A=local\n") {
		t.Fatalf("A was not preserved: %q", got)
	}
	// C should be commented out, not deleted.
	if !strings.Contains(got, "# C=old\n") {
		t.Fatalf("C was not commented out: %q", got)
	}
	// Already-commented #D should remain unchanged.
	if !strings.Contains(got, "#D=already_commented\n") {
		t.Fatalf("#D was unexpectedly modified: %q", got)
	}
	// B should be added.
	if !strings.Contains(got, "B=2\n") {
		t.Fatalf("B was not added: %q", got)
	}
	// Verify report details.
	if len(report.commentedKeys) != 1 || report.commentedKeys[0] != "C" {
		t.Fatalf("expected commentedKeys=[C], got %v", report.commentedKeys)
	}
	if len(report.addedKeys) != 1 || report.addedKeys[0] != "B" {
		t.Fatalf("expected addedKeys=[B], got %v", report.addedKeys)
	}
}
