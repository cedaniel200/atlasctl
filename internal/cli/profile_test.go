package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/i18n"
)

func TestProfileDetectProposesOnly(t *testing.T) {
	project := t.TempDir()
	writeCLIFile(t, project, "go.mod", "module example\n")

	out, errOut, code := runProject(t, project, "--lang", "en", "profile", "detect")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	catalog := i18n.New(i18n.English)
	for id, args := range map[string]map[string]string{
		MsgProfileDetected:     {"id": "go"},
		MsgProfileEvidence:     {"evidence": "go.mod"},
		MsgProfileProposalOnly: nil,
	} {
		if want := catalog.T(id, args); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".atlas", "profiles", "go.yaml")); !os.IsNotExist(err) {
		t.Errorf("proposal wrote a profile: %v", err)
	}
}

func TestProfileDetectWriteRequiresState(t *testing.T) {
	project := t.TempDir()
	writeCLIFile(t, project, "go.mod", "module example\n")

	_, errOut, code := runProject(t, project, "profile", "detect", "--write")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if want := i18n.New(i18n.English).T(MsgProfileNotConfigured, nil); !strings.Contains(errOut, want) {
		t.Errorf("stderr = %q, want %q", errOut, want)
	}
}

func TestProfileDetectWrite(t *testing.T) {
	project := profileCLIProject(t)
	out, errOut, code := runProject(t, project, "profile", "detect", "--write")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	catalog := i18n.New(i18n.English)
	if want := catalog.T(MsgProfileWritten, map[string]string{
		"path": ".atlas/profiles/go.yaml", "id": "go",
	}); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	if written := cliState(t, project); written.Profile != "go" {
		t.Errorf("state.Profile = %q, want go", written.Profile)
	}
	data, err := os.ReadFile(filepath.Join(project, ".atlas", "profiles", "go.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(profile) error = %v", err)
	}
	if !strings.Contains(string(data), "name: go") {
		t.Errorf("profile = %q, want name go", data)
	}
}

func TestProfileDetectExistingWithoutForce(t *testing.T) {
	project := profileCLIProject(t)
	if _, _, code := runProject(t, project, "profile", "detect", "--write"); code != ExitOK {
		t.Fatalf("first write exit code = %d", code)
	}
	before, err := os.ReadFile(filepath.Join(project, ".atlas", "profiles", "go.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(profile) error = %v", err)
	}

	out, _, code := runProject(t, project, "profile", "detect", "--write")
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if want := i18n.New(i18n.English).T(MsgProfileExists, map[string]string{"path": ".atlas/profiles/go.yaml"}); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	after, err := os.ReadFile(filepath.Join(project, ".atlas", "profiles", "go.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(profile) error = %v", err)
	}
	if string(before) != string(after) {
		t.Error("existing profile changed without force")
	}

	out, _, code = runProject(t, project, "profile", "detect")
	if code != ExitOK {
		t.Errorf("proposal-only exit code = %d, want %d", code, ExitOK)
	}
	catalog := i18n.New(i18n.English)
	for id, args := range map[string]map[string]string{
		MsgProfileExists:       {"path": ".atlas/profiles/go.yaml"},
		MsgProfileProposalOnly: nil,
	} {
		if want := catalog.T(id, args); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	}
}

func TestProfileDetectForce(t *testing.T) {
	project := profileCLIProject(t)
	writeCLIFile(t, project, ".atlas/profiles/go.yaml", "custom\n")
	if _, _, code := runProject(t, project, "profile", "detect", "--force"); code != ExitOK {
		t.Fatalf("force exit code = %d, want %d", code, ExitOK)
	}
	data, err := os.ReadFile(filepath.Join(project, ".atlas", "profiles", "go.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(profile) error = %v", err)
	}
	if !strings.Contains(string(data), "name: go") || strings.Contains(string(data), "custom") {
		t.Errorf("profile = %q, want it regenerated", data)
	}
}

func TestProfileDetectInteractive(t *testing.T) {
	t.Run("write", func(t *testing.T) {
		project := profileCLIProject(t)
		_, errOut, code := runProjectInteractive(t, project, "y\n", "profile", "detect")
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d: %s", code, ExitOK, errOut)
		}
		if written := cliState(t, project); written.Profile != "go" {
			t.Errorf("state.Profile = %q, want go", written.Profile)
		}
	})

	t.Run("keep existing", func(t *testing.T) {
		project := profileCLIProject(t)
		if _, _, code := runProject(t, project, "profile", "detect", "--write"); code != ExitOK {
			t.Fatalf("initial write exit code = %d", code)
		}
		before, err := os.ReadFile(filepath.Join(project, ".atlas", "profiles", "go.yaml"))
		if err != nil {
			t.Fatalf("ReadFile(profile) error = %v", err)
		}
		out, errOut, code := runProjectInteractive(t, project, "n\n", "profile", "detect")
		if code != ExitOK {
			t.Fatalf("exit code = %d, want %d: %s", code, ExitOK, errOut)
		}
		if want := i18n.New(i18n.English).T(MsgProfileProposalOnly, nil); !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q", out, want)
		}
		after, err := os.ReadFile(filepath.Join(project, ".atlas", "profiles", "go.yaml"))
		if err != nil {
			t.Fatalf("ReadFile(profile) error = %v", err)
		}
		if string(before) != string(after) {
			t.Error("profile changed after declining overwrite")
		}
	})
}

func TestProfileDetectNoMarker(t *testing.T) {
	project := t.TempDir()
	out, errOut, code := runProject(t, project, "profile", "detect")
	if code != ExitOK {
		t.Fatalf("exit code = %d, want %d: %s", code, ExitOK, errOut)
	}
	if want := i18n.New(i18n.English).T(MsgProfileNone, nil); !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestProfileUsageAndUnknownFlag(t *testing.T) {
	project := t.TempDir()
	for _, args := range [][]string{
		{"profile"},
		{"profile", "nope"},
		{"profile", "detect", "--nope"},
	} {
		_, errOut, code := runProject(t, project, args...)
		if code != ExitUsage {
			t.Errorf("args %v exit code = %d, want %d", args, code, ExitUsage)
		}
		if errOut == "" {
			t.Errorf("args %v stderr is empty", args)
		}
	}
}

func profileCLIProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	if _, _, code := runProject(t, project, "create", "--host", "opencode"); code != ExitOK {
		t.Fatalf("create exit code = %d", code)
	}
	writeCLIFile(t, project, "go.mod", "module example\n")
	return project
}
