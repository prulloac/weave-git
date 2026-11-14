package provider

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/prulloac/weave-git/internal/source"
)

// withDescriptors adds generic synthetic documents and relations to a legacy
// response. Legacy fields stay populated for older core versions.
func withDescriptors(response source.Response) source.Response {
	root := response.Snapshot.Path
	repository := response.Repository
	repositoryKeywords := boundedKeywords([]string{
		"repository", "git", repository.Name, repository.Title, repository.License,
	})
	repositoryID := "_repo"
	rootScope := ""
	readme := strings.TrimSpace(repository.Readme)
	consumes := []string{}
	if readme != "" {
		consumes = append(consumes, readme)
	}
	documents := []source.Document{{
		ID:          repositoryID,
		Type:        "Repository",
		ContentKind: "blob",
		Title:       repository.Title,
		Description: repository.Description,
		Keywords:    repositoryKeywords,
		Body:        readRepositoryBody(root, readme),
		Metadata: map[string]any{
			"v":          1,
			"provider":   "git",
			"uri":        response.Snapshot.URI,
			"ref":        response.Snapshot.Ref,
			"commit":     response.Snapshot.Commit,
			"revision":   response.Snapshot.Commit,
			"host":       repository.Host,
			"owner":      repository.Owner,
			"name":       repository.Name,
			"host_kind":  repository.HostKind,
			"web_url":    repository.WebURL,
			"commit_url": repository.CommitURL,
			"readme":     readme,
			"license":    repository.License,
			"worktree":   !response.Snapshot.ReadOnly,
		},
		Scope:    &rootScope,
		Consumes: consumes,
	}}
	relations := make([]source.Relation, 0)
	type packageAggregate struct {
		name      string
		ecosystem string
		versions  map[string]bool
		scopes    map[string]bool
	}
	packages := map[string]*packageAggregate{}
	for _, project := range response.Projects {
		path := cleanProjectPath(project.Path)
		projectID := "_project"
		if path != "" {
			projectID = "_project/" + path
		}
		projectName := strings.TrimSpace(project.Name)
		if projectName == "" {
			projectName = projectID
		}
		scope := path
		keywords := []string{"project", project.Ecosystem, projectName}
		if path != "" {
			keywords = append(keywords, path)
		}
		if project.Manifest != "" {
			keywords = append(keywords, filepath.Base(project.Manifest))
		}
		for _, dependency := range project.Dependencies {
			keywords = append(keywords, dependency.Name)
		}
		documents = append(documents, source.Document{
			ID:          projectID,
			Type:        "Project",
			ContentKind: "symbol",
			Title:       projectName,
			Description: project.Description,
			Keywords:    boundedKeywords(keywords),
			Metadata: map[string]any{
				"v":            1,
				"provider":     "git",
				"uri":          response.Snapshot.URI,
				"ref":          response.Snapshot.Ref,
				"commit":       response.Snapshot.Commit,
				"path":         path,
				"name":         projectName,
				"ecosystem":    project.Ecosystem,
				"manifest":     project.Manifest,
				"dependencies": len(project.Dependencies),
			},
			Scope: &scope,
		})
		relations = append(relations, source.Relation{From: repositoryID, To: projectID, Kind: "contains"})
		for _, dependency := range project.Dependencies {
			name := strings.TrimSpace(dependency.Name)
			if name == "" {
				continue
			}
			packageID := packageDocumentID(project.Ecosystem, name)
			aggregate := packages[packageID]
			if aggregate == nil {
				aggregate = &packageAggregate{
					name: name, ecosystem: project.Ecosystem,
					versions: map[string]bool{}, scopes: map[string]bool{},
				}
				packages[packageID] = aggregate
			}
			if dependency.Version != "" {
				aggregate.versions[dependency.Version] = true
			}
			if dependency.Scope != "" {
				aggregate.scopes[dependency.Scope] = true
			}
			relations = append(relations, source.Relation{From: projectID, To: packageID, Kind: "depends_on"})
		}
	}
	packageIDs := make([]string, 0, len(packages))
	for id := range packages {
		packageIDs = append(packageIDs, id)
	}
	sort.Strings(packageIDs)
	for _, id := range packageIDs {
		aggregate := packages[id]
		versions := sortedKeys(aggregate.versions)
		scopes := sortedKeys(aggregate.scopes)
		keywords := []string{aggregate.name, aggregate.ecosystem, "external-package"}
		keywords = append(keywords, scopes...)
		documents = append(documents, source.Document{
			ID:          id,
			Type:        "ExternalPackage",
			ContentKind: "symbol",
			Title:       aggregate.name,
			Description: aggregate.ecosystem + " dependency",
			Keywords:    boundedKeywords(keywords),
			Metadata: map[string]any{
				"v":         1,
				"name":      aggregate.name,
				"ecosystem": aggregate.ecosystem,
				"versions":  versions,
				"scopes":    scopes,
			},
		})
	}
	commitIDs := map[string]bool{}
	for _, commit := range response.Commits {
		commitIDs[commit.SHA] = true
	}
	for _, commit := range response.Commits {
		id := "_commit/" + commit.SHA
		title := strings.TrimSpace(commit.Subject)
		if title == "" {
			title = shortSHA(commit.SHA)
		}
		documents = append(documents, source.Document{
			ID:          id,
			Type:        "Commit",
			ContentKind: "symbol",
			Title:       title,
			Description: strings.TrimSpace(commit.Subject),
			Keywords:    boundedKeywords([]string{"commit", shortSHA(commit.SHA), commit.Author, title}),
			Metadata: map[string]any{
				"v":        1,
				"sha":      commit.SHA,
				"parents":  commit.Parents,
				"author":   commit.Author,
				"date":     commit.Date,
				"subject":  commit.Subject,
				"repo_uri": response.Snapshot.URI,
				"repo_ref": response.Snapshot.Ref,
			},
		})
		relations = append(relations, source.Relation{From: repositoryID, To: id, Kind: "contains"})
		for _, parent := range commit.Parents {
			if commitIDs[parent] {
				relations = append(relations, source.Relation{From: id, To: "_commit/" + parent, Kind: "parent_of"})
			}
		}
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].ID < documents[j].ID })
	relations = dedupeRelations(relations)
	response.Documents = documents
	response.Relations = relations
	response.Artifacts = repositoryArtifacts(root, documents)
	response.SourceMetadata = map[string]any{
		"provider":   "git",
		"uri":        response.Snapshot.URI,
		"ref":        response.Snapshot.Ref,
		"revision":   response.Snapshot.Commit,
		"commit_sha": response.Snapshot.Commit,
		"host":       repository.Host,
		"owner":      repository.Owner,
		"name":       repository.Name,
		"host_kind":  repository.HostKind,
		"web_url":    repository.WebURL,
		"commit_url": repository.CommitURL,
		"readme":     readme,
		"license":    repository.License,
		"worktree":   !response.Snapshot.ReadOnly,
	}
	return response
}

func repositoryArtifacts(root string, documents []source.Document) []source.Artifact {
	for _, document := range documents {
		if document.ID != "_repo" {
			continue
		}
		contains := make([]map[string]any, 0)
		readme := readmeID(document.Consumes)
		for _, file := range snapshotFileIDs(root) {
			if file == readme {
				continue
			}
			contains = append(contains, map[string]any{
				"from": "_repo", "to": file, "kind": "contains",
			})
		}
		record := map[string]any{
			"id": document.ID, "title": document.Title, "description": document.Description,
			"keywords": document.Keywords, "body": document.Body,
			"content_metadata": document.Metadata,
		}
		if len(contains) > 0 {
			record["contains"] = contains
		}
		return []source.Artifact{{
			Path: "_sources/repositories.json", MediaType: "application/json",
			Content: map[string]any{
				"v":            "weave.repositories/v1",
				"repositories": []any{record},
			},
		}}
	}
	return nil
}

func snapshotFileIDs(root string) []string {
	result := make([]string, 0)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		id := filepath.ToSlash(relative)
		if strings.HasSuffix(strings.ToLower(id), ".md") {
			id = strings.TrimSuffix(id, filepath.Ext(id))
		}
		result = append(result, id)
		return nil
	})
	sort.Strings(result)
	return result
}

func readmeID(consumes []string) string {
	if len(consumes) == 0 {
		return ""
	}
	value := filepath.ToSlash(consumes[0])
	return strings.TrimSuffix(value, filepath.Ext(value))
}

func readRepositoryBody(root, readme string) string {
	if readme == "" || filepath.IsAbs(readme) {
		return ""
	}
	clean := filepath.Clean(filepath.FromSlash(readme))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(root, clean))
	if err != nil {
		return ""
	}
	return string(data)
}

func cleanProjectPath(path string) string {
	value := strings.TrimSpace(filepath.ToSlash(path))
	value = strings.TrimPrefix(value, "./")
	value = strings.Trim(value, "/")
	if value == "" {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return ""
	}
	return clean
}

func packageDocumentID(ecosystem, name string) string {
	ecosystem = strings.ToLower(strings.TrimSpace(ecosystem))
	if ecosystem == "" {
		ecosystem = "unknown"
	}
	return "_package/" + ecosystem + "/" + packageSlug(name)
}

func packageSlug(name string) string {
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "unknown"
	}
	if len(slug) > 96 {
		slug = slug[:96]
	}
	return slug
}

func boundedKeywords(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		result = append(result, value)
		if len(result) == 64 {
			break
		}
	}
	return result
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func dedupeRelations(values []source.Relation) []source.Relation {
	seen := map[string]bool{}
	result := make([]source.Relation, 0, len(values))
	for _, value := range values {
		key := value.From + "\x00" + value.To + "\x00" + value.Kind
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].From != result[j].From {
			return result[i].From < result[j].From
		}
		if result[i].To != result[j].To {
			return result[i].To < result[j].To
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}
