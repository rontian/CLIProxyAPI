package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	envKeyPattern          = regexp.MustCompile(`^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=`)
	commentedEnvKeyPattern = regexp.MustCompile(`^#\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=`)
)

type envEntry struct {
	key   string
	lines []string
}

func syncEnv(examplePath, targetPath string, dryRun bool) (*syncReport, error) {
	report := &syncReport{}

	example, ok, err := readOptional(examplePath)
	if err != nil || !ok {
		return report, err
	}
	target, ok, err := readOptional(targetPath)
	if err != nil {
		return report, err
	}
	if !ok {
		if !dryRun {
			if err := os.WriteFile(targetPath, example, 0o644); err != nil {
				return nil, fmt.Errorf("copy %s to %s: %w", examplePath, targetPath, err)
			}
		}
		report.addedKeys = append(report.addedKeys, "(created from example)")
		return report, nil
	}

	exampleLines := splitLines(example)
	targetLines := ensureTrailingNewline(splitLines(target))

	// Build set of all keys in target (both active and commented).
	targetKeys := make(map[string]struct{}, len(targetLines))
	for _, line := range targetLines {
		if key := envKey(line); key != "" {
			targetKeys[key] = struct{}{}
		}
	}

	// Build set of active keys in example.
	exampleActiveKeys := make(map[string]struct{})
	for _, entry := range envEntries(exampleLines) {
		exampleActiveKeys[entry.key] = struct{}{}
	}

	// Comment out active target keys that are not in example (in place).
	for i, line := range targetLines {
		key := activeEnvKey(line)
		if key == "" {
			continue
		}
		if _, exists := exampleActiveKeys[key]; exists {
			continue
		}
		targetLines[i] = "# " + line
		report.commentedKeys = append(report.commentedKeys, key)
	}

	// Add missing keys from example.
	var additions []string
	for _, entry := range envEntries(exampleLines) {
		if _, exists := targetKeys[entry.key]; exists {
			continue
		}
		if len(additions) > 0 && strings.TrimSpace(additions[len(additions)-1]) != "" {
			additions = append(additions, "\n")
		}
		additions = append(additions, entry.lines...)
		targetKeys[entry.key] = struct{}{}
		report.addedKeys = append(report.addedKeys, entry.key)
	}

	if !report.changed() {
		return report, nil
	}

	if len(additions) > 0 {
		if len(targetLines) > 0 && strings.TrimSpace(targetLines[len(targetLines)-1]) != "" {
			targetLines = append(targetLines, "\n")
		}
		targetLines = append(targetLines, "\n# Added from example configuration. Existing values above were not changed.\n")
		targetLines = append(targetLines, ensureTrailingNewline(additions)...)
	}

	if !dryRun {
		if err := os.WriteFile(targetPath, []byte(strings.Join(targetLines, "")), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", targetPath, err)
		}
	}
	return report, nil
}

func readOptional(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	return data, true, nil
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	parts := bytes.SplitAfter(data, []byte("\n"))
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			lines = append(lines, string(part))
		}
	}
	return lines
}

func ensureTrailingNewline(lines []string) []string {
	if len(lines) == 0 || strings.HasSuffix(lines[len(lines)-1], "\n") {
		return lines
	}
	out := append([]string(nil), lines...)
	out[len(out)-1] += "\n"
	return out
}

// envKey returns the key name from both active and commented env lines.
func envKey(line string) string {
	trimmed := strings.TrimSpace(line)
	if match := envKeyPattern.FindStringSubmatch(trimmed); len(match) > 0 {
		return match[1]
	}
	if match := commentedEnvKeyPattern.FindStringSubmatch(trimmed); len(match) > 0 {
		return match[1]
	}
	return ""
}

// activeEnvKey returns the key name only from active (uncommented) env lines.
func activeEnvKey(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return ""
	}
	if match := envKeyPattern.FindStringSubmatch(trimmed); len(match) > 0 {
		return match[1]
	}
	return ""
}

// envEntries groups example lines into entries keyed by their active or
// commented env key. Comment lines immediately preceding a key are included
// in that entry.
func envEntries(lines []string) []envEntry {
	var entries []envEntry
	pendingStart := 0
	for index, line := range lines {
		key := envKey(line)
		if key == "" {
			if strings.TrimSpace(line) == "" {
				pendingStart = index + 1
			}
			continue
		}
		entries = append(entries, envEntry{
			key:   key,
			lines: append([]string(nil), lines[pendingStart:index+1]...),
		})
		pendingStart = index + 1
	}
	return entries
}
