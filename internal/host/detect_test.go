package host

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func makeProjectDir(t *testing.T, project, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(project, name), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", name, err)
	}
}

func TestDetectReturnsSignalsAndEvidence(t *testing.T) {
	project := t.TempDir()
	makeProjectDir(t, project, ".opencode")
	makeProjectDir(t, project, ".cursor")
	makeProjectDir(t, project, ".codex")

	cases := []struct {
		name     string
		strategy Strategy
		env      map[string]string
		signals  []string
		paths    []string
	}{
		{
			name: "opencode", strategy: opencode{},
			env:     map[string]string{"OPENCODE": "1"},
			signals: []string{"OPENCODE"},
			paths:   []string{".opencode"},
		},
		{
			name: "cursor", strategy: cursor{},
			env:     map[string]string{"TERM_PROGRAM": "Cursor"},
			signals: []string{"TERM_PROGRAM=Cursor"},
			paths:   []string{".cursor"},
		},
		{
			name: "codex", strategy: codex{},
			paths: []string{".codex"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			evidence, ok := testCase.strategy.Detect(project, environment(testCase.env))
			if !ok {
				t.Fatal("Detect() = false, want evidence")
			}
			if evidence.Host != testCase.strategy.ID() {
				t.Errorf("host = %q, want %q", evidence.Host, testCase.strategy.ID())
			}
			if !reflect.DeepEqual(evidence.Signals, testCase.signals) {
				t.Errorf("signals = %v, want %v", evidence.Signals, testCase.signals)
			}
			if !reflect.DeepEqual(evidence.Paths, testCase.paths) {
				t.Errorf("paths = %v, want %v", evidence.Paths, testCase.paths)
			}
		})
	}

	if _, ok := (claude{}).Detect(t.TempDir(), environment(nil)); ok {
		t.Error("Detect() found evidence for claude in an empty project")
	}
}

func TestResolveByEnvironment(t *testing.T) {
	project := t.TempDir()
	cases := []struct {
		name string
		host string
		env  map[string]string
	}{
		{"opencode", "opencode", map[string]string{"OPENCODE": "1"}},
		{"opencode session", "opencode", map[string]string{"OPENCODE_SESSION_ID": "1"}},
		{"opencode terminal", "opencode", map[string]string{"OPENCODE_TERMINAL": "1"}},
		{"claude", "claude", map[string]string{"CLAUDECODE": "1"}},
		{"cursor trace", "cursor", map[string]string{"CURSOR_TRACE_ID": "1"}},
		{"cursor terminal", "cursor", map[string]string{"TERM_PROGRAM": "Cursor"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			resolution, err := Resolve(project, "", environment(testCase.env))
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if resolution.Host != testCase.host {
				t.Errorf("host = %q, want %q", resolution.Host, testCase.host)
			}
			if resolution.Reason != ReasonEnvironment {
				t.Errorf("reason = %q, want %q", resolution.Reason, ReasonEnvironment)
			}
			if resolution.NeedsQuestion() {
				t.Error("NeedsQuestion() = true, want false")
			}
			if len(resolution.Evidence) == 0 {
				t.Error("evidence is empty")
			}
		})
	}
}

func TestResolveByProjectEvidence(t *testing.T) {
	cases := []struct {
		host string
		dir  string
	}{
		{"opencode", ".opencode"},
		{"claude", ".claude"},
		{"cursor", ".cursor"},
		{"codex", ".codex"},
	}

	for _, testCase := range cases {
		t.Run(testCase.host, func(t *testing.T) {
			project := t.TempDir()
			makeProjectDir(t, project, testCase.dir)

			resolution, err := Resolve(project, "", environment(nil))
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if resolution.Host != testCase.host {
				t.Errorf("host = %q, want %q", resolution.Host, testCase.host)
			}
			if resolution.Reason != ReasonProject {
				t.Errorf("reason = %q, want %q", resolution.Reason, ReasonProject)
			}
		})
	}
}

func TestResolveByEnvironmentAndProject(t *testing.T) {
	project := t.TempDir()
	makeProjectDir(t, project, ".opencode")

	resolution, err := Resolve(project, "", environment(map[string]string{"OPENCODE_SESSION_ID": "1"}))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolution.Host != "opencode" {
		t.Errorf("host = %q, want opencode", resolution.Host)
	}
	if resolution.Reason != ReasonEnvironmentAndProject {
		t.Errorf("reason = %q, want %q", resolution.Reason, ReasonEnvironmentAndProject)
	}
}

func TestResolveAsksOnContradiction(t *testing.T) {
	t.Run("signal against evidence", func(t *testing.T) {
		project := t.TempDir()
		makeProjectDir(t, project, ".claude")

		resolution, err := Resolve(project, "", environment(map[string]string{"OPENCODE": "1"}))
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if !resolution.NeedsQuestion() {
			t.Error("NeedsQuestion() = false, want a contradiction")
		}
		if resolution.Reason != ReasonContradiction {
			t.Errorf("reason = %q, want %q", resolution.Reason, ReasonContradiction)
		}
		if resolution.Host != "" {
			t.Errorf("host = %q, want none", resolution.Host)
		}
		if len(resolution.Evidence) != 2 {
			t.Errorf("evidence = %v, want both hosts", resolution.Evidence)
		}
	})

	t.Run("two signals", func(t *testing.T) {
		resolution, err := Resolve(t.TempDir(), "", environment(map[string]string{"OPENCODE": "1", "CLAUDECODE": "1"}))
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if resolution.Reason != ReasonContradiction || !resolution.NeedsQuestion() {
			t.Errorf("resolution = %+v, want a contradiction", resolution)
		}
	})

	t.Run("two project directories", func(t *testing.T) {
		project := t.TempDir()
		makeProjectDir(t, project, ".opencode")
		makeProjectDir(t, project, ".claude")

		resolution, err := Resolve(project, "", environment(nil))
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if resolution.Reason != ReasonContradiction || !resolution.NeedsQuestion() {
			t.Errorf("resolution = %+v, want a contradiction", resolution)
		}
	})
}

func TestResolveAsksWithoutEvidence(t *testing.T) {
	resolution, err := Resolve(t.TempDir(), "", environment(nil))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !resolution.NeedsQuestion() {
		t.Error("NeedsQuestion() = false, want a question")
	}
	if resolution.Reason != ReasonNoEvidence {
		t.Errorf("reason = %q, want %q", resolution.Reason, ReasonNoEvidence)
	}
	if resolution.Host != "" {
		t.Errorf("host = %q, want none", resolution.Host)
	}
	if len(resolution.Evidence) != 0 {
		t.Errorf("evidence = %v, want none", resolution.Evidence)
	}
}

func TestResolveFlagWinsWithoutReadingTheEnvironment(t *testing.T) {
	project := t.TempDir()
	makeProjectDir(t, project, ".opencode")

	calls := 0
	getenv := func(name string) string {
		calls++
		return "1"
	}

	resolution, err := Resolve(project, "claude", getenv)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolution.Host != "claude" {
		t.Errorf("host = %q, want claude", resolution.Host)
	}
	if resolution.Reason != ReasonFlag {
		t.Errorf("reason = %q, want %q", resolution.Reason, ReasonFlag)
	}
	if resolution.NeedsQuestion() {
		t.Error("NeedsQuestion() = true, want false")
	}
	if calls != 0 {
		t.Errorf("getenv called %d times, want 0", calls)
	}

	if _, err := Resolve(project, "unknown", getenv); err == nil {
		t.Error("Resolve(unknown) error = nil, want an unsupported host error")
	}
	if calls != 0 {
		t.Errorf("getenv called %d times on an unknown flag, want 0", calls)
	}
}

func TestVerificationMarksUnverifiedHosts(t *testing.T) {
	want := map[string]Verification{
		"opencode": Verified,
		"claude":   Unverified,
		"cursor":   Unverified,
		"codex":    Unverified,
	}
	for _, strategy := range All() {
		if got := strategy.Verification(); got != want[strategy.ID()] {
			t.Errorf("%s Verification() = %q, want %q", strategy.ID(), got, want[strategy.ID()])
		}
	}
}
