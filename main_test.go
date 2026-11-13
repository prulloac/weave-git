package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDescribeJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--describe", "--json"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		V               string   `json:"v"`
		Name            string   `json:"name"`
		SourceProviders []string `json:"source_providers"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.V != "weave.extension/v1" || manifest.Name != "git" ||
		len(manifest.SourceProviders) != 1 || manifest.SourceProviders[0] != "git" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
}

func TestRejectsWrongProtocol(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(nil, strings.NewReader(`{"v":"weave.cli/v1","type":"source_request"}`), &stdout, &stderr)
	if err == nil {
		t.Fatal("expected protocol error")
	}
}

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "weave-git ") {
		t.Fatalf("unexpected version: %s", stdout.String())
	}
}

func TestGCJSON(t *testing.T) {
	cache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cache, "sources", "src-stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"gc", "--cache-dir", cache, "--json"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var response struct {
		V      string `json:"v"`
		Type   string `json:"type"`
		DryRun bool   `json:"dry_run"`
		Result struct {
			Removed []string `json:"removed"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.V != "weave.extension/v1" || response.Type != "gc_response" || response.DryRun {
		t.Fatalf("unexpected response: %+v", response)
	}
	if len(response.Result.Removed) != 1 {
		t.Fatalf("removed = %+v", response.Result.Removed)
	}
}

func TestGCRejectsUnknownArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"gc", "--nope"}, strings.NewReader(""), &stdout, &stderr); err == nil {
		t.Fatal("expected an argument error")
	}
}
