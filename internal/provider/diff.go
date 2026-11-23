package provider

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/prulloac/weave-git/internal/source"
)

const maxDiffPatchBytes = 12 << 20

// Diff compares two provider-resolved refs and returns structured changes.
func Diff(request source.DiffRequest) (source.DiffResponse, error) {
	if !sourceIDPattern.MatchString(request.SourceID) {
		return source.DiffResponse{}, fmt.Errorf("invalid source_id %q", request.SourceID)
	}
	loc, err := parseLocation(request.URI)
	if err != nil {
		return source.DiffResponse{}, err
	}
	root := loc.Path
	if loc.Remote {
		if strings.TrimSpace(request.CacheDir) == "" {
			return source.DiffResponse{}, errors.New("cache_dir is required")
		}
		root, err = ensureMirror(request.CacheDir, loc)
		if err != nil {
			return source.DiffResponse{}, err
		}
	}
	from, err := resolveCommit(root, request.From)
	if err != nil {
		return source.DiffResponse{}, err
	}
	to, err := resolveCommit(root, request.To)
	if err != nil {
		return source.DiffResponse{}, err
	}
	status, err := gitBytes(root, "diff", "--name-status", "-z", "-M", "--no-ext-diff", from, to)
	if err != nil {
		return source.DiffResponse{}, err
	}
	changes, err := parseNameStatus(status)
	if err != nil {
		return source.DiffResponse{}, err
	}
	binary, err := binaryPaths(root, from, to)
	if err != nil {
		return source.DiffResponse{}, err
	}
	for index := range changes {
		if binary[changes[index].Path] {
			changes[index].Binary = true
		}
	}

	response := source.DiffResponse{
		V:       source.ProtocolVersion,
		Type:    "source_diff_response",
		From:    from,
		To:      to,
		Changes: changes,
	}
	if request.Options.Format != "patch" {
		return response, nil
	}
	context := request.Options.Context
	if context <= 0 {
		context = 3
	}
	total := 0
	for index := range response.Changes {
		change := &response.Changes[index]
		if change.Binary {
			continue
		}
		patch, err := changePatch(root, from, to, context, *change)
		if err != nil {
			return source.DiffResponse{}, err
		}
		if total+len(patch) > maxDiffPatchBytes {
			response.Truncated = true
			break
		}
		total += len(patch)
		change.Patch = patch
	}
	return response, nil
}

func parseNameStatus(data []byte) ([]source.DiffChange, error) {
	tokens := bytes.Split(data, []byte{0})
	changes := make([]source.DiffChange, 0)
	for index := 0; index < len(tokens); {
		status := string(tokens[index])
		index++
		if strings.TrimSpace(status) == "" {
			continue
		}
		if index >= len(tokens) {
			return nil, errors.New("malformed diff response")
		}
		switch status[0] {
		case 'A':
			changes = append(changes, source.DiffChange{Kind: "added", Path: string(tokens[index])})
			index++
		case 'D':
			changes = append(changes, source.DiffChange{Kind: "removed", Path: string(tokens[index])})
			index++
		case 'M', 'T':
			changes = append(changes, source.DiffChange{Kind: "modified", Path: string(tokens[index])})
			index++
		case 'R':
			if index+1 >= len(tokens) {
				return nil, errors.New("malformed diff response")
			}
			changes = append(changes, source.DiffChange{
				Kind:         "renamed",
				Path:         string(tokens[index+1]),
				PreviousPath: string(tokens[index]),
			})
			index += 2
		case 'C':
			if index+1 >= len(tokens) {
				return nil, errors.New("malformed diff response")
			}
			changes = append(changes, source.DiffChange{Kind: "added", Path: string(tokens[index+1])})
			index += 2
		default:
			return nil, errors.New("malformed diff response")
		}
	}
	return changes, nil
}

func binaryPaths(root, from, to string) (map[string]bool, error) {
	output, err := gitBytes(root, "diff", "--numstat", "--no-renames", "--no-ext-diff", from, to)
	if err != nil {
		return nil, err
	}
	binary := make(map[string]bool)
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 || parts[0] != "-" || parts[1] != "-" {
			continue
		}
		binary[parts[2]] = true
	}
	return binary, nil
}

func changePatch(root, from, to string, context int, change source.DiffChange) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "-M", "-U" + strconv.Itoa(context), from, to, "--"}
	if change.Kind == "renamed" {
		args = append(args, change.PreviousPath)
	}
	args = append(args, change.Path)
	output, err := gitBytes(root, args...)
	if err != nil {
		return "", err
	}
	return string(output), nil
}
