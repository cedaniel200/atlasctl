package host

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/plan"
)

// contractFile is the shared contract every host reads; it is always an allowed
// route even when the host does not generate it.
const contractFile = "AGENTS.md"

var descriptionLine = regexp.MustCompile(`(?m)^>\s*(.+)$`)

type sourceFile struct {
	bundle string
	name   string
	data   []byte
}

func validateInput(input Input) error {
	if input.Snapshot == nil {
		return errors.New("host: nil snapshot")
	}
	if len(uniqueSorted(input.Bundles)) == 0 {
		return errors.New("host: at least one active bundle is required")
	}
	return nil
}

// sourceFiles collects the direct markdown children of the source directory a
// bundle declares for roles or commands. Nested files and README.md are not
// source material.
func sourceFiles(input Input, pick func(content.Bundle) string) ([]sourceFile, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	bundles := make(map[string]content.Bundle, len(input.Snapshot.Bundles()))
	for _, bundle := range input.Snapshot.Bundles() {
		bundles[bundle.ID] = bundle
	}

	var files []sourceFile
	for _, id := range uniqueSorted(input.Bundles) {
		bundle, ok := bundles[id]
		if !ok {
			return nil, fmt.Errorf("host: active bundle %q is not in the snapshot", id)
		}
		directory := strings.TrimSuffix(strings.TrimSpace(pick(bundle)), "/")
		if directory == "" {
			continue
		}
		if err := fsutil.ValidateRelative(directory); err != nil {
			return nil, fmt.Errorf("host: bundle %q declares an invalid source directory %q: %w", id, directory, err)
		}

		prefix := directory + "/"
		for _, file := range input.Snapshot.Files() {
			if file.Bundle != id || !strings.HasPrefix(file.Path, prefix) {
				continue
			}
			relative := strings.TrimPrefix(file.Path, prefix)
			if path.Dir(relative) != "." || !strings.HasSuffix(relative, ".md") {
				continue
			}
			name := strings.TrimSuffix(relative, ".md")
			if name == "" || name == "README" {
				continue
			}
			files = append(files, sourceFile{bundle: id, name: name, data: file.Data})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].name != files[j].name {
			return files[i].name < files[j].name
		}
		return files[i].bundle < files[j].bundle
	})
	return files, nil
}

// renderAgents prepends the host frontmatter to the canonical role body. Role
// files in the bundle carry only the body, never their own YAML frontmatter.
func renderAgents(input Input, id string, caps Capabilities, frontmatter func(name, description string) string) ([]plan.Generated, error) {
	if caps.AgentsDir == "" {
		return nil, fmt.Errorf("host: %q declares no agents directory", id)
	}
	files, err := sourceFiles(input, func(bundle content.Bundle) string { return bundle.Roles })
	if err != nil {
		return nil, err
	}

	generated := make([]plan.Generated, 0, len(files))
	for _, file := range files {
		data := append([]byte(frontmatter(file.name, describe(file.data, file.name))), file.data...)
		generated = append(generated, plan.Generated{
			Path:   path.Join(caps.AgentsDir, file.name+".md"),
			Bundle: file.bundle,
			Origin: "host:" + id,
			Data:   data,
		})
	}
	return generated, nil
}

// renderCommands copies command markdown as published. Its frontmatter is valid
// for both current hosts; host-specific command shaping is not needed yet.
func renderCommands(input Input, id string, caps Capabilities) ([]plan.Generated, error) {
	if caps.CommandsDir == "" {
		return nil, fmt.Errorf("host: %q declares no commands directory", id)
	}
	files, err := sourceFiles(input, func(bundle content.Bundle) string { return bundle.Commands })
	if err != nil {
		return nil, err
	}

	generated := make([]plan.Generated, 0, len(files))
	for _, file := range files {
		generated = append(generated, plan.Generated{
			Path:   path.Join(caps.CommandsDir, file.name+".md"),
			Bundle: file.bundle,
			Origin: "host:" + id,
			Data:   file.data,
		})
	}
	return generated, nil
}

func describe(data []byte, name string) string {
	if match := descriptionLine.FindSubmatch(data); match != nil {
		return strings.TrimSpace(string(match[1]))
	}
	return "Rol " + name + " del arnés"
}

func allowedRoutes(caps Capabilities, legacy []string) []string {
	routes := []string{
		contractFile,
		caps.AgentsDir,
		caps.CommandsDir,
		caps.StartupFile,
		caps.MCPFile,
	}
	routes = append(routes, legacy...)
	return uniqueSorted(routes)
}

func configFiles(caps Capabilities) []string {
	if caps.MCPFile == "" {
		return nil
	}
	return []string{caps.MCPFile}
}

func primaryBundle(bundles []string) string {
	ids := uniqueSorted(bundles)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}
