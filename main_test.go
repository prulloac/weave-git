package main

import (
	"bytes"
	"encoding/json"
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
