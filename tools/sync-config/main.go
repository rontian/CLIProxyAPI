package main

import (
	"flag"
	"fmt"
	"os"
)

type options struct {
	config        string
	configExample string
	env           string
	envExample    string
	skipYAML      bool
	skipEnv       bool
	dryRun        bool
}

func main() {
	opts := options{}
	flag.StringVar(&opts.config, "config", "config.yaml", "local YAML config path")
	flag.StringVar(&opts.configExample, "config-example", "config.example.yaml", "example YAML config path")
	flag.StringVar(&opts.env, "env", ".env", "local env path")
	flag.StringVar(&opts.envExample, "env-example", ".env.example", "example env path")
	flag.BoolVar(&opts.skipYAML, "skip-yaml", false, "skip YAML config sync")
	flag.BoolVar(&opts.skipEnv, "skip-env", false, "skip env sync")
	flag.BoolVar(&opts.dryRun, "dry-run", false, "preview changes without writing")
	flag.Parse()

	type fileResult struct {
		path   string
		report *syncReport
	}
	var results []fileResult

	if !opts.skipYAML {
		report, err := syncConfig(opts.configExample, opts.config, opts.dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sync config: %v\n", err)
			os.Exit(1)
		}
		if report.changed() {
			results = append(results, fileResult{path: opts.config, report: report})
		}
	}
	if !opts.skipEnv {
		report, err := syncEnv(opts.envExample, opts.env, opts.dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sync env: %v\n", err)
			os.Exit(1)
		}
		if report.changed() {
			results = append(results, fileResult{path: opts.env, report: report})
		}
	}

	if len(results) == 0 {
		fmt.Println("config files are already up to date")
		return
	}

	prefix := "updated"
	if opts.dryRun {
		prefix = "would update"
	}
	for _, r := range results {
		fmt.Printf("%s: %s\n", prefix, r.path)
		for _, key := range r.report.addedKeys {
			fmt.Printf("  + %s\n", key)
		}
		for _, key := range r.report.commentedKeys {
			fmt.Printf("  # %s (commented out, not in example)\n", key)
		}
	}
}
