package provider

import (
	"fmt"
	"strings"

	"github.com/prulloac/weave-git/internal/source"
)

const (
	defaultHistoryLimit = 20
	maxHistoryLimit     = 100
)

// readHistory returns commits reachable from the resolved commit, newest first.
func readHistory(root, commit string, limit int) ([]source.Commit, error) {
	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}
	output, err := git(root, "log",
		fmt.Sprintf("--max-count=%d", limit),
		"--format=%H%x1f%P%x1f%an%x1f%aI%x1f%s%x1e",
		commit,
	)
	if err != nil {
		return nil, err
	}
	records := strings.Split(output, "\x1e")
	commits := make([]source.Commit, 0, len(records))
	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		fields := strings.Split(record, "\x1f")
		if len(fields) < 5 {
			continue
		}
		parents := make([]string, 0)
		for _, parent := range strings.Fields(fields[1]) {
			if parent != "" {
				parents = append(parents, parent)
			}
		}
		commits = append(commits, source.Commit{
			SHA:     strings.TrimSpace(fields[0]),
			Parents: parents,
			Author:  strings.TrimSpace(fields[2]),
			Date:    strings.TrimSpace(fields[3]),
			Subject: strings.TrimSpace(fields[4]),
		})
	}
	return commits, nil
}

// enrichRepository adds deterministic host URLs without calling a hosting API.
func enrichRepository(repository *source.Repository, loc location, commit string) {
	if repository == nil || !loc.Remote || strings.TrimSpace(loc.Host) == "" {
		return
	}
	host := strings.TrimSpace(loc.Host)
	repository.HostKind = hostKind(host)
	repository.WebURL = "https://" + host + "/" + repositoryPath(loc)
	switch repository.HostKind {
	case "gitlab":
		repository.CommitURL = repository.WebURL + "/-/commit/" + commit
	case "bitbucket":
		repository.CommitURL = repository.WebURL + "/commits/" + commit
	default:
		repository.CommitURL = repository.WebURL + "/commit/" + commit
	}
}

func repositoryPath(loc location) string {
	owner := strings.Trim(loc.Owner, "/")
	name := strings.Trim(loc.Name, "/")
	if owner == "" {
		return name
	}
	return owner + "/" + name
}

func hostKind(host string) string {
	switch {
	case host == "github.com" || strings.Contains(strings.ToLower(host), "github"):
		return "github"
	case host == "gitlab.com" || strings.Contains(strings.ToLower(host), "gitlab"):
		return "gitlab"
	case host == "bitbucket.org" || strings.Contains(strings.ToLower(host), "bitbucket"):
		return "bitbucket"
	default:
		return "generic"
	}
}
