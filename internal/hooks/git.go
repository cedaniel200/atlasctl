package hooks

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// isRepositoryRoot reports whether project is the root of a Git worktree. A
// project nested inside a larger repository is rejected: core.hooksPath is
// relative to the repository root and installing from a subdirectory would
// configure the wrong path.
func isRepositoryRoot(project string) bool {
	toplevel, err := gitOutput(project, "rev-parse", "--show-toplevel")
	if err != nil || toplevel == "" {
		return false
	}
	projectInfo, err := os.Stat(project)
	if err != nil {
		return false
	}
	topInfo, err := os.Stat(toplevel)
	if err != nil {
		return false
	}
	return os.SameFile(projectInfo, topInfo)
}

func gitOutput(project string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func configuredHooksPath(project string) (string, bool, error) {
	cmd := exec.Command("git", "-C", project, "config", "--get", "core.hooksPath")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return strings.TrimSpace(stdout.String()), true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", false, nil
	}
	return "", false, fmt.Errorf("git config core.hooksPath: %w: %s", err, strings.TrimSpace(stderr.String()))
}

func setLocalHooksPath(project, value string) error {
	cmd := exec.Command("git", "-C", project, "config", "--local", "core.hooksPath", value)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hooks: set core.hooksPath: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
