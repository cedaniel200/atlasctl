package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/commands"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func TestCheckWithoutStateViaCLI(t *testing.T) {
	project := t.TempDir()
	_, errOut, code := runProject(t, project, "--lang", "es", "check")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if want := i18n.New(i18n.Spanish).T(MsgCheckNotConfigured, nil); !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
}

func TestCheckSucceedsWithoutRecommendations(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	out, errOut, code := runProject(t, project, "--lang", "es", "check")
	if code != ExitOK {
		t.Fatalf("check exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if want := i18n.New(i18n.Spanish).T(MsgCheckSummaryOK, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestCheckReportsPendingAndDispatcherPermission(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	dispatcher := filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))
	if err := os.Chmod(dispatcher, 0o644); err != nil {
		t.Fatalf("Chmod(dispatcher) error = %v", err)
	}

	out, errOut, code := runProject(t, project, "--lang", "en", "check")
	if code != ExitFindings {
		t.Fatalf("check exit code = %d, want %d: %s", code, ExitFindings, errOut)
	}
	catalog := i18n.New(i18n.English)
	for id, args := range map[string]map[string]string{
		MsgCheckStatePending:         nil,
		MsgCheckDispatcherPermission: nil,
		MsgCheckSummaryFindings:      {"count": "5"},
	} {
		if want := catalog.T(id, args); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	}
}

func TestCheckDistinguishesDispatcherContentAndPermission(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project, "create", "--host", "opencode")
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	dispatcher := filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))
	if err := os.Chmod(dispatcher, 0o644); err != nil {
		t.Fatalf("Chmod(dispatcher) error = %v", err)
	}

	out, _, code := runProject(t, project, "--lang", "en", "check")
	if code != ExitFindings {
		t.Fatalf("check exit code = %d, want %d", code, ExitFindings)
	}
	if want := i18n.New(i18n.English).T(MsgCheckDispatcherPermission, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want the permission message %q", out, want)
	}

	if err := os.WriteFile(dispatcher, []byte("#!/bin/sh\necho edited\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(dispatcher) error = %v", err)
	}
	out, _, code = runProject(t, project, "--lang", "en", "check")
	if code != ExitFindings {
		t.Fatalf("check exit code = %d, want %d", code, ExitFindings)
	}
	catalog := i18n.New(i18n.English)
	if want := catalog.T(MsgCheckDispatcherModified, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want the edited-content message %q", out, want)
	}
	if !strings.Contains(out, catalog.T(MsgCheckNo, nil)) && strings.Contains(out, catalog.T(MsgCheckDispatcherPermission, nil)) {
		t.Errorf("stdout = %q, want the permission message replaced", out)
	}
}

func TestCheckUnreadableStateIsError(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".atlas"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.atlas) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, ".atlas", "state.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}

	_, errOut, code := runProject(t, project, "check")
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if errOut == "" {
		t.Error("stderr is empty, want the inspection error")
	}
}

func TestApplyRestoresDispatcherAndCheckConfirms(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	dispatcher := filepath.Join(project, filepath.FromSlash(hooks.DispatcherPath))
	if err := os.Chmod(dispatcher, 0o644); err != nil {
		t.Fatalf("Chmod(dispatcher) error = %v", err)
	}

	if _, _, code := runProject(t, project, "--lang", "es", "check"); code != ExitFindings {
		t.Fatalf("check exit code = %d, want %d", code, ExitFindings)
	}
	if _, errOut, code := runProject(t, project, "--lang", "es", "apply"); code != ExitOK {
		t.Fatalf("apply exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	info, err := os.Stat(dispatcher)
	if err != nil {
		t.Fatalf("Stat(dispatcher) error = %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("dispatcher mode = %o, want 0755", info.Mode().Perm())
	}
	out, errOut, code := runProject(t, project, "--lang", "es", "check")
	if code != ExitOK {
		t.Fatalf("check exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if want := i18n.New(i18n.Spanish).T(MsgCheckDispatcherOK, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestDoctorViaCLI(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	out, errOut, code := runProject(t, project, "--lang", "es", "doctor")
	if code != ExitOK {
		t.Fatalf("doctor exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	catalog := i18n.New(i18n.Spanish)
	lines := []struct {
		id   string
		args map[string]string
	}{
		{MsgDoctorTitle, nil},
		{MsgDoctorBinary, map[string]string{"version": commands.EngineVersion}},
		{MsgDoctorState, map[string]string{"schema": "2", "engine": commands.EngineVersion}},
		{MsgDoctorHost, map[string]string{"id": "opencode", "source": catalog.T("doctor.source.flag", nil)}},
		{MsgDoctorSpecNone, nil},
		{MsgDoctorSetupComplete, nil},
		{MsgDoctorProfileNone, nil},
		{MsgDoctorNextNone, nil},
	}
	for _, line := range lines {
		if want := catalog.T(line.id, line.args); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	}
}

func TestDoctorExplainHostViaCLI(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.claude) error = %v", err)
	}
	if _, _, code := runProject(t, project, "create", "--host", "opencode"); code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	out, errOut, code := runProject(t, project, "--lang", "en", "doctor", "--explain-host")
	if code != ExitOK {
		t.Fatalf("doctor exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	catalog := i18n.New(i18n.English)
	if want := catalog.T(MsgDoctorExplainHeader, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	for _, id := range []string{"opencode", "claude", "cursor", "codex"} {
		if !strings.Contains(out, id+": signals ") {
			t.Errorf("stdout = %q, want the evidence line for %s", out, id)
		}
	}
	if want := catalog.T(MsgDoctorExplainItem, map[string]string{
		"id": "claude", "signals": catalog.T(MsgCheckNone, nil), "paths": ".claude",
		"verification": catalog.T("doctor.verification.unverified", nil),
	}); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestDoctorTranslatesMachineValues(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.claude) error = %v", err)
	}
	if _, _, code := runProject(t, project, "create", "--host", "opencode"); code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	out, errOut, code := runProject(t, project, "--lang", "es", "doctor")
	if code != ExitOK {
		t.Fatalf("doctor exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	catalog := i18n.New(i18n.Spanish)
	for _, id := range []string{"doctor.source.flag", "doctor.reason.project", "doctor.verification.unverified"} {
		if want := catalog.T(id, nil); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	}
	for _, raw := range []string{"flag (", "project (", "unverified"} {
		if strings.Contains(out, raw) {
			t.Errorf("stdout = %q, want the machine value %q translated", out, raw)
		}
	}
}

func TestDoctorHooksLineIsComplete(t *testing.T) {
	project := t.TempDir()
	if _, _, code := runProject(t, project, "create", "--host", "opencode"); code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}

	out, _, code := runProject(t, project, "--lang", "en", "doctor")
	if code != ExitOK {
		t.Fatalf("doctor exit code = %d, want %d", code, ExitOK)
	}
	catalog := i18n.New(i18n.English)
	want := catalog.T(MsgDoctorHooks, map[string]string{
		"repository": catalog.T(MsgCheckNo, nil),
		"path":       "unset",
		"dispatcher": catalog.T(MsgCheckNo, nil),
		"guards":     catalog.T(MsgCheckNo, nil),
		"active":     catalog.T(MsgCheckNo, nil),
	})
	if !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestDoctorReportsManifestMissingAndNextApply(t *testing.T) {
	project := t.TempDir()
	_, _, code := runProject(t, project,
		"--lang", "es", "create",
		"--host", "opencode", "--docs", "docs", "--no-decisions", "--bundle", "dev",
	)
	if code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	if err := os.Remove(filepath.Join(project, ".atlas", "manifest.json")); err != nil {
		t.Fatalf("Remove(manifest) error = %v", err)
	}

	out, _, code := runProject(t, project, "--lang", "es", "doctor")
	if code != ExitOK {
		t.Fatalf("doctor exit code = %d, want %d", code, ExitOK)
	}
	catalog := i18n.New(i18n.Spanish)
	for _, id := range []string{MsgDoctorManifestMissing, MsgDoctorNextApply} {
		if want := catalog.T(id, nil); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	}
}

func TestDoctorReportsInvalidSpecWithoutFailing(t *testing.T) {
	project := t.TempDir()
	if _, _, code := runProject(t, project, "create", "--host", "opencode"); code != ExitOK {
		t.Fatalf("create exit code = %d, want %d", code, ExitOK)
	}
	written := cliState(t, project)
	written.SpecEngine = state.SpecEngine{ID: "unknown"}
	if err := state.Save(filepath.Join(project, ".atlas", "state.json"), written); err != nil {
		t.Fatalf("state.Save() error = %v", err)
	}

	out, errOut, code := runProject(t, project, "--lang", "es", "doctor")
	if code != ExitOK {
		t.Fatalf("doctor exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	catalog := i18n.New(i18n.Spanish)
	if !strings.Contains(out, "unknown") {
		t.Errorf("stdout = %q, want the invalid adapter named", out)
	}
	if want := catalog.T(MsgDoctorNextReview, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestDoctorWithoutStateViaCLI(t *testing.T) {
	project := t.TempDir()
	_, errOut, code := runProject(t, project, "doctor")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if want := i18n.New(i18n.English).T(MsgDoctorNotConfigured, nil); !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
}

func TestDoctorUnknownFlagIsUsage(t *testing.T) {
	project := t.TempDir()
	_, errOut, code := runProject(t, project, "doctor", "--nope")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if want := i18n.New(i18n.English).T(MsgUnknownFlag, map[string]string{"flag": "--nope"}); !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
}

func TestDoctorUnreadableStateIsError(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".atlas"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.atlas) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, ".atlas", "state.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}

	_, errOut, code := runProject(t, project, "doctor")
	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if errOut == "" {
		t.Error("stderr is empty, want the inspection error")
	}
}
