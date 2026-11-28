package setup

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func emptyEnv(string) string { return "" }

func makeDir(t *testing.T, project, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(project, filepath.FromSlash(name)), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", name, err)
	}
}

type scriptedPrompter struct {
	t         *testing.T
	answers   []string
	questions []string
	fail      bool
}

func (p *scriptedPrompter) Ask(question, defaultValue string) (string, error) {
	if p.fail {
		p.t.Errorf("unexpected prompt: %q", question)
		return "", errors.New("unexpected prompt")
	}
	p.questions = append(p.questions, question)
	if len(p.answers) == 0 {
		return defaultValue, nil
	}
	answer := p.answers[0]
	p.answers = p.answers[1:]
	return answer, nil
}

func TestResolveLeavesAllQuestionsPendingWithoutAnswers(t *testing.T) {
	project := t.TempDir()
	result, err := Resolve(Request{Project: project, Getenv: emptyEnv, CLILanguage: "en"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	want := []string{PendingHost, PendingLanguage, PendingDocs, PendingDecisions, PendingBundles}
	if !reflect.DeepEqual(result.Pending, want) {
		t.Errorf("pending = %v, want %v", result.Pending, want)
	}
	if result.Complete {
		t.Error("Complete = true, want false")
	}
	if result.State.Host.ID != "" {
		t.Errorf("host = %q, want empty", result.State.Host.ID)
	}
	if want := []string{DefaultBundle}; !reflect.DeepEqual(result.State.Bundles, want) {
		t.Errorf("bundles = %v, want %v", result.State.Bundles, want)
	}
	if result.State.SpecEngine.ID != "none" {
		t.Errorf("spec engine = %q, want none", result.State.SpecEngine.ID)
	}
	if result.State.Language.CLI != "en" || result.State.Language.User != "en" || result.State.Language.Agent != "en" {
		t.Errorf("language = %+v, want en/en/en", result.State.Language)
	}
	if result.State.Layout.Docs != "" || result.State.Layout.Decisions != nil {
		t.Errorf("layout = %+v, want both routes absent", result.State.Layout)
	}
	if !reflect.DeepEqual(result.State.Setup.Pending, want) || result.State.Setup.Complete {
		t.Errorf("state setup = %+v, want pending %v", result.State.Setup, want)
	}
}

func TestResolveDetectsHostAndDocs(t *testing.T) {
	project := t.TempDir()
	makeDir(t, project, ".opencode")
	makeDir(t, project, "docs")

	result, err := Resolve(Request{Project: project, Getenv: emptyEnv, CLILanguage: "en"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.State.Host.ID != "opencode" {
		t.Errorf("host = %q, want opencode", result.State.Host.ID)
	}
	if result.State.Host.Source != "evidence" {
		t.Errorf("host source = %q, want evidence", result.State.Host.Source)
	}
	if !contains(result.State.Host.Evidence, ".opencode") {
		t.Errorf("host evidence = %v, want .opencode", result.State.Host.Evidence)
	}
	if result.State.Layout.Docs != "docs" {
		t.Errorf("docs = %q, want docs", result.State.Layout.Docs)
	}
	want := []string{PendingLanguage, PendingDecisions, PendingBundles}
	if !reflect.DeepEqual(result.Pending, want) {
		t.Errorf("pending = %v, want %v", result.Pending, want)
	}
}

func TestResolveFlagsAnswerAllQuestions(t *testing.T) {
	project := t.TempDir()
	prompter := &scriptedPrompter{t: t, fail: true}
	result, err := Resolve(Request{
		Project:     project,
		Answers:     Answers{Host: "opencode", Docs: "documentation", NoDecisions: true, Bundles: []string{"dev"}, Profile: "go"},
		Interactive: true,
		Prompter:    prompter,
		Catalog:     i18n.New(i18n.English),
		Getenv:      emptyEnv,
		CLILanguage: "es",
		LangSet:     true,
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(result.Pending) != 0 || !result.Complete {
		t.Errorf("result = %+v, want complete", result)
	}
	if result.State.Host.ID != "opencode" || result.State.Host.Source != "flag" {
		t.Errorf("host = %+v, want opencode by flag", result.State.Host)
	}
	if result.State.Language.User != "es" || result.State.Language.CLI != "es" {
		t.Errorf("language = %+v, want es", result.State.Language)
	}
	if result.State.Layout.Docs != "documentation" || result.State.Layout.Decisions != nil {
		t.Errorf("layout = %+v, want documentation and no decisions", result.State.Layout)
	}
	if result.State.Profile != "go" {
		t.Errorf("profile = %q, want go", result.State.Profile)
	}
	if len(prompter.questions) != 0 {
		t.Errorf("questions = %v, want none", prompter.questions)
	}
}

func TestResolveAsksOnlyAmbiguousQuestions(t *testing.T) {
	project := t.TempDir()
	makeDir(t, project, ".opencode")
	makeDir(t, project, "docs")
	prompter := &scriptedPrompter{t: t, answers: []string{"es", "", "dev"}}

	result, err := Resolve(Request{
		Project:     project,
		Interactive: true,
		Prompter:    prompter,
		Catalog:     i18n.New(i18n.English),
		Getenv:      emptyEnv,
		CLILanguage: "en",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(prompter.questions) != 3 {
		t.Errorf("asked %d questions, want 3 (language, decisions, bundles)", len(prompter.questions))
	}
	if !result.Complete {
		t.Errorf("result = %+v, want complete", result)
	}
	if result.State.Host.ID != "opencode" || result.State.Layout.Docs != "docs" {
		t.Errorf("detected host/docs were not kept: %+v", result.State)
	}
	if result.State.Language.User != "es" {
		t.Errorf("language = %+v, want the interview answer", result.State.Language)
	}
	if result.State.Layout.Decisions != nil {
		t.Errorf("decisions = %v, want absent", result.State.Layout.Decisions)
	}
}

func TestResolveContradictionLeavesHostPending(t *testing.T) {
	project := t.TempDir()
	makeDir(t, project, ".claude")

	result, err := Resolve(Request{
		Project: project,
		Getenv: func(name string) string {
			if name == "OPENCODE" {
				return "1"
			}
			return ""
		},
		CLILanguage: "en",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.State.Host.ID != "" {
		t.Errorf("host = %q, want pending", result.State.Host.ID)
	}
	if !contains(result.Pending, PendingHost) {
		t.Errorf("pending = %v, want host", result.Pending)
	}
}

func TestResolveMergesExistingState(t *testing.T) {
	existing := &state.State{
		SchemaVersion: state.SchemaVersion,
		Host:          state.Host{ID: "opencode", Source: "flag"},
		Language:      state.Language{CLI: "es", User: "es", Agent: "en"},
		Layout:        state.Layout{Docs: "documentation"},
		Bundles:       []string{"dev"},
		SpecEngine:    state.SpecEngine{ID: "none"},
		Setup:         state.Setup{Pending: []string{PendingBundles}},
	}

	result, err := Resolve(Request{
		Project:     t.TempDir(),
		Existing:    existing,
		Answers:     Answers{Bundles: []string{"dev"}},
		Getenv:      emptyEnv,
		CLILanguage: "es",
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !result.Complete || len(result.Pending) != 0 {
		t.Errorf("result = %+v, want complete", result)
	}
	if result.State.Host.ID != "opencode" || result.State.Language.User != "es" {
		t.Errorf("state = %+v, want the existing host and language", result.State)
	}
	if result.State.Layout.Docs != "documentation" {
		t.Errorf("docs = %q, want the existing route", result.State.Layout.Docs)
	}
}

func TestResolveRejectsConflictingOrMissingInputs(t *testing.T) {
	cases := []struct {
		name    string
		request Request
	}{
		{"docs conflict", Request{Project: t.TempDir(), Answers: Answers{Docs: "docs", NoDocs: true}}},
		{"decisions conflict", Request{Project: t.TempDir(), Answers: Answers{Decisions: "adr", NoDecisions: true}}},
		{"no project", Request{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := Resolve(testCase.request); err == nil {
				t.Fatal("Resolve() error = nil, want an input error")
			}
		})
	}
}

func TestResolveDetectsProfile(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	result, err := Resolve(Request{Project: project, Getenv: emptyEnv, CLILanguage: "en"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.State.Profile != "go" {
		t.Errorf("profile = %q, want go", result.State.Profile)
	}
}

func TestResolveDetectsOpenSpecRoot(t *testing.T) {
	project := t.TempDir()
	makeDir(t, project, "openspec")
	result, err := Resolve(Request{Project: project, Getenv: emptyEnv, CLILanguage: "en"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result.State.SpecEngine.ID != "openspec" || result.State.SpecEngine.Root != "openspec" {
		t.Errorf("spec engine = %+v, want openspec", result.State.SpecEngine)
	}
}

func TestTerminalReturnsDefaultOnEmptyOrEOF(t *testing.T) {
	var out strings.Builder
	terminal := NewTerminal(strings.NewReader("value\n\n"), &out)

	answer, err := terminal.Ask("question? ", "default")
	if err != nil || answer != "value" {
		t.Errorf("Ask() = (%q, %v), want value", answer, err)
	}
	answer, err = terminal.Ask("question? ", "default")
	if err != nil || answer != "default" {
		t.Errorf("Ask(empty) = (%q, %v), want default", answer, err)
	}
	answer, err = terminal.Ask("question? ", "default")
	if err != nil || answer != "default" {
		t.Errorf("Ask(EOF) = (%q, %v), want default", answer, err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
