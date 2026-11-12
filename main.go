package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

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
					"v":                "weave.extension/v1",
					"name":             "git",
					"source_providers": []string{"git"},
					"capabilities":     []string{"repository", "ref-resolution"},
				})
			}
			fmt.Fprintln(stdout, "name: git")
			fmt.Fprintln(stdout, "source providers: git")
			fmt.Fprintln(stdout, "capabilities: repository, ref-resolution")
			return nil
		}
	}
	var request source.Request
	if err := json.NewDecoder(stdin).Decode(&request); err != nil {
		return fmt.Errorf("decode source request: %w", err)
	}
	response, err := provider.Materialize(request)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(response)
}
