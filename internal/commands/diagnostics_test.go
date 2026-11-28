package commands

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/setup"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func TestCheckReportsPendingSetupAsState(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	report, err := Check(CheckRequest{Project: project})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	wantIDs := []string{
		setup.PendingHost, setup.PendingLanguage, setup.PendingDocs,
		setup.PendingDecisions, setup.PendingBundles, SetupProfile,
	}
	if len(report.Setup) != len(wantIDs) {
		t.Fatalf("setup = %+v, want the six identifiers", report.Setup)
	}
	for index, id := range wantIDs {
		if report.Setup[index].ID != id {
			t.Errorf("setup[%d].ID = %q, want %q", index, report.Setup[index].ID, id)
		}
	}
	for _, item := range report.Setup[:5] {
		if !item.Pending {
			t.Errorf("%s is not pending", item.ID)
		}
	}
	if report.Setup[5].Pending {
		t.Error("profile must never be a setup question")
	}
	if report.Recommendations != 5 {
		t.Errorf("recommendations = %d, want the five pending questions", report.Recommendations)
	}
	if len(report.Files) != 0 {
		t.Errorf("files = %+v, want no drift in a fresh project", report.Files)
	}
	if report.Dispatcher == nil || !report.Dispatcher.Tracked || !report.Dispatcher.Executable {
		t.Errorf("dispatcher = %+v, want it tracked and executable", report.Dispatcher)
	}
}

func TestCheckDetectsBaseDrift(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writeProjectFile(t, project, "AGENTS.md", "edited by the adopter\n")

	report, err := Check(CheckRequest{Project: project})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	var found bool
	for _, file := range report.Files {
		if file.Path == "AGENTS.md" {
			found = true
			if file.Layer != state.LayerBase || file.Status != "modified" {
				t.Errorf("file = %+v, want a modified base file", file)
			}
		}
	}
	if !found {
		t.Errorf("files = %+v, want AGENTS.md reported", report.Files)
	}
}

func TestCheckDistinguishesDispatcherContentFromPermission(t *testing.T) {
	t.Run("permission lost", func(t *testing.T) {
		project := t.TempDir()
		if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		dispatcher := filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))
		if err := os.Chmod(dispatcher, 0o644); err != nil {
			t.Fatalf("Chmod() error = %v", err)
		}

		report, err := Check(CheckRequest{Project: project})
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if report.Dispatcher == nil {
			t.Fatal("dispatcher is nil")
		}
		if report.Dispatcher.Status != "intact" || report.Dispatcher.Executable {
			t.Errorf("dispatcher = %+v, want intact without the executable bit", report.Dispatcher)
		}
	})

	t.Run("content edited", func(t *testing.T) {
		project := t.TempDir()
		if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		dispatcher := filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))
		if err := os.WriteFile(dispatcher, []byte("#!/bin/sh\necho edited\n"), 0o755); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		report, err := Check(CheckRequest{Project: project})
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if report.Dispatcher == nil {
			t.Fatal("dispatcher is nil")
		}
		if report.Dispatcher.Status != "modified" {
			t.Errorf("dispatcher = %+v, want modified content", report.Dispatcher)
		}
	})
}

func TestCheckReportsMissingManifest(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := os.Remove(filepath.Join(project, ".atlas", "manifest.json")); err != nil {
		t.Fatalf("Remove(manifest) error = %v", err)
	}

	report, err := Check(CheckRequest{Project: project})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if report.Manifest != nil {
		t.Errorf("manifest = %+v, want nil", report.Manifest)
	}
	if report.Dispatcher == nil || report.Dispatcher.Tracked {
		t.Errorf("dispatcher = %+v, want it reported as untracked", report.Dispatcher)
	}
}

func TestCheckWithoutState(t *testing.T) {
	if _, err := Check(CheckRequest{Project: t.TempDir()}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Check() error = %v, want ErrNotConfigured", err)
	}
}

func TestCheckReportsActiveHooks(t *testing.T) {
	project := initDiagnosticsRepo(t)
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	report, err := Check(CheckRequest{Project: project})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !report.Hooks.Active || report.HooksRecommended {
		t.Errorf("hooks = %+v, want active and no recommendation", report.Hooks)
	}
}

func initDiagnosticsRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	root := t.TempDir()
	runDiagnosticsGit(t, root, "init", "--quiet")
	runDiagnosticsGit(t, root, "symbolic-ref", "HEAD", "refs/heads/main")
	runDiagnosticsGit(t, root, "config", "user.name", "Diagnostics Test")
	runDiagnosticsGit(t, root, "config", "user.email", "diagnostics@example.test")
	writeProjectFile(t, root, "README.md", "# repo\n")
	runDiagnosticsGit(t, root, "add", "README.md")
	runDiagnosticsGit(t, root, "commit", "--quiet", "--no-verify", "-m", "chore: base")
	return root
}

func runDiagnosticsGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestCheckDetectsBaseAndGeneratedAbsence(t *testing.T) {
	snapshot := t.TempDir()
	writeProjectFile(t, snapshot, "dev/bundle.json", `{"id":"dev","version":"0.1.0","requires":[],"roles":"roles","commands":"commands"}`)
	writeProjectFile(t, snapshot, "dev/AGENTS.md", "# Contract\n")
	writeProjectFile(t, snapshot, "dev/roles/analyst.md", "# analyst\n")
	writeProjectFile(t, snapshot, "dev/commands/apply.md", "---\ndescription: Apply\n---\n\nBody.\n")

	project := t.TempDir()
	createRequest := request(project, setup.Answers{Host: "opencode", Bundles: []string{"dev"}})
	createRequest.Snapshot = snapshot
	if _, err := Create(createRequest); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	for _, rel := range []string{"AGENTS.md", ".opencode/agents/analyst.md"} {
		if err := os.Remove(filepath.Join(project, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("Remove(%s) error = %v", rel, err)
		}
	}

	report, err := Check(CheckRequest{Project: project})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	want := map[string]state.Layer{
		"AGENTS.md":                   state.LayerBase,
		".opencode/agents/analyst.md": state.LayerGenerated,
	}
	for _, file := range report.Files {
		layer, ok := want[file.Path]
		if !ok {
			continue
		}
		if file.Status != "absent" || file.Layer != layer {
			t.Errorf("file = %+v, want absent %s", file, layer)
		}
		delete(want, file.Path)
	}
	if len(want) != 0 {
		t.Errorf("missing files = %v, want both absences reported", want)
	}
}

func TestDoctorReportsHostEvidence(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.claude) error = %v", err)
	}

	report, err := Doctor(DoctorRequest{Project: project, Getenv: emptyEnv})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.State.EngineVersion == "" || report.State.SchemaVersion != state.SchemaVersion {
		t.Errorf("state versions = %+v, want schema and engine", report.State)
	}
	if report.Host.ID != "opencode" {
		t.Errorf("host = %q, want the stored host", report.Host.ID)
	}
	if report.Resolution.Host != "claude" || report.Resolution.Reason != "project" {
		t.Errorf("resolution = %+v, want claude from project evidence", report.Resolution)
	}
	if report.ResolutionVerification != "unverified" {
		t.Errorf("verification = %q, want unverified", report.ResolutionVerification)
	}
	if len(report.Hosts) != 4 {
		t.Fatalf("hosts = %+v, want every registered host", report.Hosts)
	}
	var claude HostEvidence
	for _, item := range report.Hosts {
		if item.ID == "claude" {
			claude = item
		}
	}
	if len(claude.Paths) != 1 || claude.Paths[0] != ".claude" {
		t.Errorf("claude evidence = %+v, want the .claude path", claude)
	}
}

func TestDoctorReportsSpecCLIApplicability(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "openspec"), 0o755); err != nil {
		t.Fatalf("MkdirAll(openspec) error = %v", err)
	}
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	report, err := Doctor(DoctorRequest{Project: project, Getenv: emptyEnv})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.Spec.ID != "openspec" || report.Spec.Root != "openspec" {
		t.Errorf("spec = %+v, want openspec", report.Spec)
	}
	if !report.Spec.CLIApplicable {
		t.Error("openspec CLI applicability = false, want true")
	}
	if report.Spec.Error != "" {
		t.Errorf("spec error = %q, want none", report.Spec.Error)
	}
}

func TestDoctorReportsInvalidSpecInsteadOfFailing(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	written := readState(t, project)
	written.SpecEngine = state.SpecEngine{ID: "unknown"}
	if err := state.Save(filepath.Join(project, ".atlas", "state.json"), written); err != nil {
		t.Fatalf("state.Save() error = %v", err)
	}

	report, err := Doctor(DoctorRequest{Project: project, Getenv: emptyEnv})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.Spec.Error == "" {
		t.Error("spec error = empty, want the unknown adapter reported")
	}
	if report.NextAction != "review" {
		t.Errorf("next action = %q, want review", report.NextAction)
	}
}

func TestDoctorRecommendsTheNextAction(t *testing.T) {
	t.Run("configure", func(t *testing.T) {
		project := t.TempDir()
		if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		report, err := Doctor(DoctorRequest{Project: project, Getenv: emptyEnv})
		if err != nil {
			t.Fatalf("Doctor() error = %v", err)
		}
		if report.NextAction != "configure" {
			t.Errorf("next action = %q, want configure", report.NextAction)
		}
	})

	t.Run("apply", func(t *testing.T) {
		project := t.TempDir()
		req := request(project, setup.Answers{
			Host: "opencode", Docs: "docs", NoDecisions: true, Bundles: []string{"dev"},
		})
		req.LangSet = true
		req.CLILanguage = "es"
		if _, err := Create(req); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := os.Remove(filepath.Join(project, ".atlas", "manifest.json")); err != nil {
			t.Fatalf("Remove(manifest) error = %v", err)
		}
		report, err := Doctor(DoctorRequest{Project: project, Getenv: emptyEnv})
		if err != nil {
			t.Fatalf("Doctor() error = %v", err)
		}
		if report.NextAction != "apply" {
			t.Errorf("next action = %q, want apply", report.NextAction)
		}
	})

	t.Run("none", func(t *testing.T) {
		project := t.TempDir()
		req := request(project, setup.Answers{
			Host: "opencode", Docs: "docs", NoDecisions: true, Bundles: []string{"dev"},
		})
		req.LangSet = true
		req.CLILanguage = "es"
		if _, err := Create(req); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		report, err := Doctor(DoctorRequest{Project: project, Getenv: emptyEnv})
		if err != nil {
			t.Fatalf("Doctor() error = %v", err)
		}
		if report.NextAction != "none" {
			t.Errorf("next action = %q, want none", report.NextAction)
		}
	})
}

func TestDoctorDoesNotWrite(t *testing.T) {
	project := t.TempDir()
	if _, err := Create(request(project, setup.Answers{Host: "opencode"})); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	statePath := filepath.Join(project, ".atlas", "state.json")
	manifestPath := filepath.Join(project, ".atlas", "manifest.json")
	stateBefore, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state) error = %v", err)
	}
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}

	if _, err := Doctor(DoctorRequest{Project: project, Getenv: emptyEnv}); err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	stateAfter, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile(state) after doctor error = %v", err)
	}
	if string(stateBefore) != string(stateAfter) {
		t.Error("doctor changed the state")
	}
	manifestAfter, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) after doctor error = %v", err)
	}
	if string(manifestBefore) != string(manifestAfter) {
		t.Error("doctor changed the manifest")
	}
}

func TestDoctorWithoutState(t *testing.T) {
	if _, err := Doctor(DoctorRequest{Project: t.TempDir(), Getenv: emptyEnv}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Doctor() error = %v, want ErrNotConfigured", err)
	}
}
