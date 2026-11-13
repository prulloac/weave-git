package provider

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	schemePattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
	scpLikePattern  = regexp.MustCompile(`^([^@/:]+)@([^/:]+):(.+)$`)
	fullCommitRef   = regexp.MustCompile(`^[0-9a-fA-F]{40}$|^[0-9a-fA-F]{64}$`)
	hexCommitishRef = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
)

// location is a parsed git source URI or local path.
type location struct {
	URI      string
	FetchURI string
	Path     string
	Remote   bool
	Host     string
	Owner    string
	Name     string
}

func parseLocation(uri string) (location, error) {
	value := strings.TrimSpace(uri)
	if value == "" {
		return location{}, errors.New("source uri is required")
	}
	if strings.HasPrefix(value, "git+") {
		value = strings.TrimPrefix(value, "git+")
	}
	if strings.HasPrefix(value, "file://") {
		return remoteLocation(value)
	}
	if schemePattern.MatchString(value) {
		return remoteLocation(value)
	}
	if match := scpLikePattern.FindStringSubmatch(value); len(match) == 4 {
		path := strings.TrimLeft(match[3], "/")
		owner, name := splitRepositoryPath(path)
		if name == "" {
			return location{}, fmt.Errorf("invalid git source %q", uri)
		}
		return location{
			URI:      fmt.Sprintf("ssh://%s@%s/%s", match[1], match[2], path),
			FetchURI: fmt.Sprintf("ssh://%s@%s/%s", match[1], match[2], path),
			Remote:   true,
			Host:     match[2],
			Owner:    owner,
			Name:     name,
		}, nil
	}
	return localLocation(value)
}

func localLocation(path string) (location, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return location{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return location{}, err
	}
	if !info.IsDir() {
		return location{}, fmt.Errorf("source is not a directory: %s", absolute)
	}
	return location{URI: absolute, FetchURI: absolute, Path: absolute, Name: filepath.Base(absolute)}, nil
}

func remoteLocation(value string) (location, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return location{}, err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "http", "ssh", "git", "file":
	default:
		return location{}, fmt.Errorf("unsupported git source scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" && strings.ToLower(parsed.Scheme) != "file" {
		return location{}, fmt.Errorf("git source has no host: %s", value)
	}
	if parsed.User != nil {
		parsed.User = url.User(parsed.User.Username())
	}
	fetchURI := value
	path := strings.Trim(parsed.Path, "/")
	owner, name := splitRepositoryPath(path)
	if name == "" {
		return location{}, fmt.Errorf("git source has no repository path: %s", value)
	}
	return location{
		URI:      parsed.String(),
		FetchURI: fetchURI,
		Remote:   true,
		Host:     parsed.Hostname(),
		Owner:    owner,
		Name:     name,
	}, nil
}

func splitRepositoryPath(path string) (string, string) {
	segments := make([]string, 0)
	for _, segment := range strings.Split(path, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	if len(segments) == 0 {
		return "", ""
	}
	name := strings.TrimSuffix(segments[len(segments)-1], ".git")
	return strings.Join(segments[:len(segments)-1], "/"), name
}

// mirrorPath returns the bare mirror location for a remote source.
func (l location) mirrorPath(cacheDir string) (string, error) {
	if !l.Remote {
		return "", errors.New("local sources have no mirror")
	}
	if strings.TrimSpace(cacheDir) == "" {
		return "", errors.New("cache_dir is required")
	}
	parts := []string{cacheDir, "git", sanitizeSegment(l.Host)}
	owner := make([]string, 0)
	for _, segment := range strings.Split(l.Owner, "/") {
		if segment != "" {
			owner = append(owner, sanitizeSegment(segment))
		}
	}
	if len(owner) == 0 {
		owner = append(owner, "_")
	}
	parts = append(parts, owner...)
	parts = append(parts, sanitizeSegment(l.Name)+".git")
	mirror := filepath.Join(parts...)
	relative, err := filepath.Rel(cacheDir, mirror)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("mirror path escapes cache: %s", mirror)
	}
	return mirror, nil
}

func sanitizeSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return "_"
	}
	var builder strings.Builder
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '.' || char == '_' || char == '-' {
			builder.WriteRune(char)
			continue
		}
		builder.WriteByte('_')
	}
	if builder.Len() == 0 {
		return "_"
	}
	return builder.String()
}

func normalizeRef(ref string) string {
	value := strings.TrimSpace(ref)
	if value == "" {
		return "HEAD"
	}
	return value
}
