package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var topLevelKeyPattern = regexp.MustCompile(`^(?:#\s*)?([A-Za-z0-9_-]+):(?:\s|$)`)

// syncReport captures detailed change information from a sync operation.
type syncReport struct {
	addedKeys     []string
	commentedKeys []string
}

func (r *syncReport) changed() bool {
	return len(r.addedKeys) > 0 || len(r.commentedKeys) > 0
}

type yamlBlock struct {
	key   string
	start int
	end   int
	lines []string
}

func syncConfig(examplePath, targetPath string, dryRun bool) (*syncReport, error) {
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
	exampleBlocks := yamlBlocks(exampleLines)
	targetBlocks := yamlBlocks(targetLines)

	exampleKeys := make(map[string]struct{}, len(exampleBlocks))
	for _, block := range exampleBlocks {
		exampleKeys[block.key] = struct{}{}
	}

	// Comment out blocks present in target (active) but not in example.
	for i, block := range targetBlocks {
		if _, exists := exampleKeys[block.key]; exists {
			continue
		}
		// Only comment out if the key line is not already commented.
		if isActiveYamlKey(block.lines[0]) {
			for j := block.start; j < block.end; j++ {
				line := targetLines[j]
				if strings.TrimSpace(line) == "" {
					continue
				}
				targetLines[j] = "# " + line
			}
			report.commentedKeys = append(report.commentedKeys, block.key)
			targetBlocks[i].lines = append([]string(nil), targetLines[block.start:block.end]...)
		}
	}

	// Recompute target blocks after potential commenting changes.
	targetBlocks = yamlBlocks(targetLines)
	targetKeys := make(map[string]struct{}, len(targetBlocks))
	for _, block := range targetBlocks {
		targetKeys[block.key] = struct{}{}
	}

	// Add missing blocks from example, preserving relative order.
	for blockIndex, block := range exampleBlocks {
		if _, exists := targetKeys[block.key]; exists {
			continue
		}
		insertAt := len(targetLines)
		for previousIndex := blockIndex - 1; previousIndex >= 0; previousIndex-- {
			previous := exampleBlocks[previousIndex]
			for _, targetBlock := range targetBlocks {
				if targetBlock.key == previous.key {
					insertAt = targetBlock.end
					break
				}
			}
			if insertAt != len(targetLines) {
				break
			}
		}

		addition := append([]string(nil), block.lines...)
		// Strip trailing blank lines from the addition to avoid double
		// blank lines when the previous block already ends with one.
		for len(addition) > 0 && strings.TrimSpace(addition[len(addition)-1]) == "" {
			addition = addition[:len(addition)-1]
		}
		if insertAt > 0 && strings.TrimSpace(targetLines[insertAt-1]) != "" {
			addition = append([]string{"\n"}, addition...)
		}
		addition = ensureTrailingNewline(addition)
		targetLines = append(targetLines[:insertAt], append(addition, targetLines[insertAt:]...)...)
		targetBlocks = yamlBlocks(targetLines)
		targetKeys[block.key] = struct{}{}
		report.addedKeys = append(report.addedKeys, block.key)
	}

	if report.changed() && !dryRun {
		if err := os.WriteFile(targetPath, []byte(strings.Join(targetLines, "")), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", targetPath, err)
		}
	}
	return report, nil
}

// yamlKeyAny returns the top-level key name from any line (active or commented).
// Only matches keys at column 0 (not indented) to distinguish top-level blocks
// from nested YAML mappings.
func yamlKeyAny(line string) string {
	if len(line) == 0 {
		return ""
	}
	if line[0] == ' ' || line[0] == '\t' {
		return ""
	}
	match := topLevelKeyPattern.FindStringSubmatch(line)
	if len(match) == 0 {
		return ""
	}
	return match[1]
}

// isActiveYamlKey returns true if the line is an active (uncommented) top-level YAML key.
func isActiveYamlKey(line string) bool {
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return false
	}
	return yamlKeyAny(line) != ""
}

func yamlBlocks(lines []string) []yamlBlock {
	starts := make([]yamlBlock, 0)
	for index, line := range lines {
		if key := yamlKeyAny(line); key != "" {
			starts = append(starts, yamlBlock{key: key, start: index})
		}
	}
	blocks := make([]yamlBlock, 0, len(starts))
	for index, block := range starts {
		end := len(lines)
		if index+1 < len(starts) {
			end = starts[index+1].start
		}
		block.end = end
		block.lines = append([]string(nil), lines[block.start:end]...)
		blocks = append(blocks, block)
	}
	return blocks
}
