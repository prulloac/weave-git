package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prulloac/weave-git/internal/provider"
	"github.com/prulloac/weave-git/internal/source"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "weave-git:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "--version", "-v", "version":
			fmt.Fprintf(stdout, "weave-git %s (weave.extension/v1)\n", version)
			return nil
		case "--describe":
			jsonMode := len(args) > 1 && args[1] == "--json"
			if jsonMode {
				return json.NewEncoder(stdout).Encode(map[string]any{
					"v":                   "weave.extension/v1",
					"name":                "git",
					"source_providers":    []string{"git"},
					"capabilities":        []string{"repository", "ref-resolution", "cache-gc"},
					"document_namespaces": []string{"_repo", "_project", "_package/", "_commit/"},
					"artifact_namespaces": []string{"_sources/"},
					"relation_kinds": []map[string]any{
						{"name": "depends_on", "default": true},
						{"name": "parent_of", "default": true},
					},
				})
			}
			fmt.Fprintln(stdout, "name: git")
			fmt.Fprintln(stdout, "source providers: git")
			fmt.Fprintln(stdout, "capabilities: repository, ref-resolution, cache-gc")
			fmt.Fprintln(stdout, "document namespaces: _repo, _project, _package/, _commit/")
			fmt.Fprintln(stdout, "relation kinds: depends_on, parent_of")
			return nil
		case "gc":
			return runGC(args[1:], stdout, stderr)
		case "--help", "-h", "help":
			printUsage(stdout)
			return nil
		}
	}
	var request source.Request
	if err := json.NewDecoder(stdin).Decode(&request); err != nil {
		return fmt.Errorf("decode source request: %w", err)
	}
	if request.V != source.ProtocolVersion || request.Type != "source_request" {
		return fmt.Errorf("unsupported source request")
	}
	response, err := provider.Materialize(request)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(response)
}

func runGC(args []string, stdout, stderr io.Writer) error {
	cacheDir := strings.TrimSpace(os.Getenv("WEAVE_CACHE_DIR"))
	jsonOut := false
	dryRun := false
	keep := make([]string, 0)
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--cache-dir":
			if index+1 >= len(args) {
				return fmt.Errorf("--cache-dir requires a value")
			}
			cacheDir = args[index+1]
			index++
		case "--keep":
			if index+1 >= len(args) {
				return fmt.Errorf("--keep requires a source id")
			}
			keep = append(keep, args[index+1])
			index++
		case "--dry-run":
			dryRun = true
		case "--json":
			jsonOut = true
		case "--help", "-h":
			printGCUsage(stdout)
			return nil
		default:
			return fmt.Errorf("unknown gc argument %q", args[index])
		}
	}
	result, err := provider.CollectGarbage(provider.GCOptions{CacheDir: cacheDir, Keep: keep, DryRun: dryRun})
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(stdout).Encode(map[string]any{
			"v":       "weave.extension/v1",
			"type":    "gc_response",
			"dry_run": dryRun,
			"result":  result,
		})
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(stderr, "warning:", warning)
	}
	fmt.Fprintf(stdout, "removed %d, pruned %d\n", len(result.Removed), len(result.Pruned))
	return nil
}

func printUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, "usage: weave-git [--version|--describe [--json]|gc|help]")
	fmt.Fprintln(stdout, "source requests are read as weave.source/v1 JSON on stdin")
}

func printGCUsage(stdout io.Writer) {
	fmt.Fprintln(stdout, "usage: weave-git gc [--cache-dir <dir>] [--keep <source-id> ...] [--dry-run] [--json]")
}
