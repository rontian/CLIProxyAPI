package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncConfigAddsMissingTopLevelBlocksOnly(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, "config.example.yaml")
	target := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(example, []byte("port: 8317\n\nplugins:\n  enabled: false\n\nauto-router:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("port: 9999\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := syncConfig(example, target, false)
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
	if !strings.Contains(got, "port: 9999\n") {
		t.Fatalf("existing value overwritten: %q", got)
	}
	if !strings.Contains(got, "plugins:\n  enabled: false\n") || !strings.Contains(got, "auto-router:\n  enabled: false\n") {
		t.Fatalf("missing added blocks: %q", got)
	}
	if len(report.addedKeys) != 2 {
		t.Fatalf("expected 2 added keys, got %d: %v", len(report.addedKeys), report.addedKeys)
	}
	if len(report.commentedKeys) != 0 {
		t.Fatalf("expected 0 commented keys, got %d: %v", len(report.commentedKeys), report.commentedKeys)
	}
}

func TestSyncConfigDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, "config.example.yaml")
	target := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(example, []byte("port: 8317\nplugins:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("port: 9999\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := syncConfig(example, target, true)
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
	if got := string(data); got != "port: 9999\n" {
		t.Fatalf("dry run wrote target: %q", got)
	}
}

func TestSyncConfigCommentsOutRemovedBlocks(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, "config.example.yaml")
	target := filepath.Join(dir, "config.yaml")
	// Example has only port and plugins.
	if err := os.WriteFile(example, []byte("port: 8317\n\nplugins:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Target has port, plugins, and an extra block "legacy" not in example.
	if err := os.WriteFile(target, []byte("port: 9999\n\nplugins:\n  enabled: true\n\nlegacy:\n  mode: old\n  flag: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := syncConfig(example, target, false)
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
	// Existing values should be preserved.
	if !strings.Contains(got, "port: 9999\n") {
		t.Fatalf("existing port value overwritten: %q", got)
	}
	if !strings.Contains(got, "plugins:\n  enabled: true\n") {
		t.Fatalf("existing plugins value overwritten: %q", got)
	}
	// The "legacy" block should be commented out, not deleted.
	if !strings.Contains(got, "# legacy:") {
		t.Fatalf("legacy block not commented out: %q", got)
	}
	if !strings.Contains(got, "#   mode: old\n") {
		t.Fatalf("legacy block content not commented out: %q", got)
	}
	// The key "legacy" should appear in commentedKeys.
	if len(report.commentedKeys) != 1 || report.commentedKeys[0] != "legacy" {
		t.Fatalf("expected commentedKeys=[legacy], got %v", report.commentedKeys)
	}
	// No new blocks should be added since example keys already exist in target.
	if len(report.addedKeys) != 0 {
		t.Fatalf("expected 0 added keys, got %d: %v", len(report.addedKeys), report.addedKeys)
	}
}
