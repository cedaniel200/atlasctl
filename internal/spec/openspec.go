package spec

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/state"
)

const openSpecID = "openspec"

type openSpecAdapter struct {
	project string
	root    string
}

func resolveOpenSpec(engine state.SpecEngine, project string) (Adapter, error) {
	if project == "" {
		return nil, errors.New("spec: the openspec adapter requires a project root")
	}
	root := strings.TrimSuffix(strings.TrimSpace(engine.Root), "/")
	if root == "" {
		return nil, errors.New("spec: openspec requires a declared root")
	}
	if err := fsutil.ValidateRelative(root); err != nil {
		return nil, fmt.Errorf("spec: invalid openspec root %q: %w", engine.Root, err)
	}
	return openSpecAdapter{project: project, root: root}, nil
}

func (a openSpecAdapter) ID() string { return openSpecID }

func (a openSpecAdapter) Root() string { return a.root }

func (a openSpecAdapter) Validate(change string) error {
	return a.run("validate", change, "--no-interactive", "--type", "change")
}

func (a openSpecAdapter) Archive(change string) error {
	return a.run("archive", change, "--yes")
}

// Sync delegates to the CLI. OpenSpec 1.5 does not expose a sync subcommand
// (the sync workflow is agent-driven), so the CLI error is returned as is; a
// future CLI with sync works without changing the adapter.
func (a openSpecAdapter) Sync(change string) error {
	return a.run("sync", change)
}

func (a openSpecAdapter) run(operation, change string, flags ...string) error {
	if err := validateChange(change); err != nil {
		return err
	}
	binary, err := exec.LookPath("openspec")
	if err != nil {
		return fmt.Errorf(
			"spec: the openspec CLI is not installed or not on PATH; install it and make sure `openspec --version` works before running %s (atlasctl does not download tools)",
			operation,
		)
	}

	args := append([]string{operation}, flags...)
	args = append(args, change)
	cmd := exec.Command(binary, args...)
	cmd.Dir = a.workingDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"spec: openspec %s %q failed: %w: %s",
			operation, change, err, strings.TrimSpace(string(output)),
		)
	}
	return nil
}

// workingDir is the directory the OpenSpec CLI resolves its root from: the
// parent of the declared root. For the conventional "openspec" root that is
// the project itself.
func (a openSpecAdapter) workingDir() string {
	parent := filepath.Dir(filepath.FromSlash(a.root))
	if parent == "." {
		return a.project
	}
	return filepath.Join(a.project, parent)
}

func validateChange(change string) error {
	if change == "" {
		return errors.New("spec: change name is required")
	}
	if strings.HasPrefix(change, "-") {
		return fmt.Errorf("spec: invalid change name %q", change)
	}
	return nil
}
