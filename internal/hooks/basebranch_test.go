package hooks

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseBranchBlocksChangeArtifacts(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	stageFile(t, root, "openspec/changes/demo/proposal.md", "# Proposal\n")

	output, code := attemptCommit(t, root)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	for _, want := range []string{
		"base branch 'main'",
		"The artifacts of change 'demo' are staged on the base branch",
		"git worktree add",
		".worktrees/demo",
		"feature/demo",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output = %q, want it to contain %q", output, want)
		}
	}
}

func TestBaseBranchBlocksOtherChanges(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	stageFile(t, root, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, root)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(output, "feature branch") || !strings.Contains(output, "pull request") {
		t.Errorf("output = %q, want a feature branch and pull request suggestion", output)
	}
	if strings.Contains(output, "git worktree add") {
		t.Errorf("output = %q, want no worktree suggestion for an unrelated change", output)
	}
}

func TestFeatureBranchAllowsTheCommit(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	runGit(t, root, "checkout", "--quiet", "-b", "feature/demo")
	stageFile(t, root, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, root)
	if code != 0 {
		t.Errorf("exit code = %d, want 0: %s", code, output)
	}
	if strings.Contains(output, "Commit blocked") {
		t.Errorf("output = %q, want an allowed commit", output)
	}
}

func TestWorktreeAllowsTheCommit(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	runGit(t, root, "add", ".atlas")
	runGit(t, root, "commit", "--quiet", "--no-verify", "-m", "chore: hooks")

	worktree := filepath.Join(t.TempDir(), "wt")
	runGit(t, root, "worktree", "add", worktree, "-b", "feature/wt")
	stageFile(t, worktree, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, worktree)
	if code != 0 {
		t.Errorf("exit code = %d, want 0: %s", code, output)
	}
}

func TestDetachedHeadAllowsTheCommit(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	runGit(t, root, "checkout", "--quiet", "--detach")
	stageFile(t, root, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, root)
	if code != 0 {
		t.Errorf("exit code = %d, want 0: %s", code, output)
	}
	if !strings.Contains(output, "detached HEAD") {
		t.Errorf("output = %q, want a detached HEAD warning", output)
	}
}

func TestUnknownBaseAllowsTheCommit(t *testing.T) {
	root := initRepo(t, "topic", true)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	stageFile(t, root, "src/app.js", "export const ok = true;\n")

	output, code := attemptCommit(t, root)
	if code != 0 {
		t.Errorf("exit code = %d, want 0: %s", code, output)
	}
	if !strings.Contains(output, "could not determine the base branch") {
		t.Errorf("output = %q, want an undeterminable base warning", output)
	}
}

func TestNoCommitsAllowsTheFirstCommit(t *testing.T) {
	root := initRepo(t, "main", false)
	installHooks(t, root, Options{SpecEngineRoot: "openspec"})
	stageFile(t, root, "README.md", "# repo\n")

	output, code := attemptCommit(t, root)
	if code != 0 {
		t.Errorf("exit code = %d, want 0: %s", code, output)
	}
	if !strings.Contains(output, "no commits yet") {
		t.Errorf("output = %q, want a no-commits warning", output)
	}
}

func TestCustomSpecEngineRootDetectsChangeArtifacts(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "specs"})
	stageFile(t, root, "specs/changes/demo/proposal.md", "# Proposal\n")

	output, code := attemptCommit(t, root)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(output, ".worktrees/demo") {
		t.Errorf("output = %q, want the change worktree suggestion", output)
	}
}

func TestSpecEngineRootIsShellQuoted(t *testing.T) {
	root := initRepo(t, "main", true)
	installHooks(t, root, Options{SpecEngineRoot: "specs'dir"})
	stageFile(t, root, "specs'dir/changes/demo/proposal.md", "# Proposal\n")

	output, code := attemptCommit(t, root)
	if code != 1 {
		t.Errorf("exit code = %d, want 1: %s", code, output)
	}
	if !strings.Contains(output, ".worktrees/demo") {
		t.Errorf("output = %q, want the change worktree suggestion", output)
	}
}
