package provider

import (
	"strconv"
	"strings"

	"github.com/prulloac/weave-git/internal/source"
)

func normalizeOptions(options source.Options) source.Options {
	if len(options.ProviderOptions) == 0 {
		return options
	}
	if value, ok := optionBool(options.ProviderOptions, "worktree"); ok {
		options.Worktree = value
	}
	if value, ok := optionBool(options.ProviderOptions, "submodules"); ok {
		options.Submodules = value
	}
	if value, ok := optionBool(options.ProviderOptions, "lfs"); ok {
		options.LFS = value
	}
	if value, ok := optionBool(options.ProviderOptions, "history"); ok {
		options.History = value
	}
	if value, ok := optionInt(options.ProviderOptions, "history_limit"); ok {
		options.HistoryLimit = value
	}
	if value, ok := optionInt(options.ProviderOptions, "depth"); ok {
		options.Depth = value
	}
	return options
}

func optionBool(values map[string]any, key string) (bool, bool) {
	value, ok := values[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return parsed, true
		}
	case float64:
		return typed != 0, true
	}
	return false, false
}

func optionInt(values map[string]any, key string) (int, bool) {
	value, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}
