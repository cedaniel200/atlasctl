package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/state"
)

func ptr(value string) *string {
	return &value
}

func TestResolveLayoutReadsDeclaredPaths(t *testing.T) {
	resolved, err := ResolveLayout(state.Layout{
		Docs:      "documentation/",
		Decisions: ptr("decisions/"),
	})
	if err != nil {
		t.Fatalf("ResolveLayout() error = %v", err)
	}
	want := Layout{
		Docs:              "documentation",
		Decisions:         "decisions",
		DocsDeclared:      true,
		DecisionsDeclared: true,
	}
	if !reflect.DeepEqual(resolved, want) {
		t.Errorf("ResolveLayout() = %+v, want %+v", resolved, want)
	}
}

func TestResolveLayoutRecordsAbsentDecisions(t *testing.T) {
	resolved, err := ResolveLayout(state.Layout{Docs: "documentation/"})
	if err != nil {
		t.Fatalf("ResolveLayout() error = %v", err)
	}
	if !resolved.DocsDeclared || resolved.Docs != "documentation" {
		t.Errorf("docs = %+v, want the declared documentation/ route", resolved)
	}
	if resolved.DecisionsDeclared || resolved.Decisions != "" {
		t.Errorf("decisions = %+v, want an absent route", resolved)
	}

	project := t.TempDir()
	if _, err := os.Stat(filepath.Join(project, "adr")); !os.IsNotExist(err) {
		t.Errorf("a project without ADR received an adr directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "docs")); !os.IsNotExist(err) {
		t.Errorf("the resolver assumed a docs directory: %v", err)
	}
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the layout resolver created entries: %v", entries)
	}
}

func TestResolveLayoutAbsentRoutes(t *testing.T) {
	resolved, err := ResolveLayout(state.Layout{})
	if err != nil {
		t.Fatalf("ResolveLayout() error = %v", err)
	}
	if resolved.DocsDeclared || resolved.DecisionsDeclared {
		t.Errorf("resolved = %+v, want both routes absent", resolved)
	}
	if resolved.Docs != "" || resolved.Decisions != "" {
		t.Errorf("resolved = %+v, want no paths", resolved)
	}
}

func TestResolveLayoutEmptyDecisionPointerIsAbsent(t *testing.T) {
	resolved, err := ResolveLayout(state.Layout{Docs: "docs", Decisions: ptr("")})
	if err != nil {
		t.Fatalf("ResolveLayout() error = %v", err)
	}
	if !resolved.DocsDeclared || resolved.DecisionsDeclared {
		t.Errorf("resolved = %+v, want docs declared and decisions absent", resolved)
	}
}

func TestResolveLayoutRejectsInvalidPaths(t *testing.T) {
	cases := []struct {
		name   string
		layout state.Layout
	}{
		{"docs traversal", state.Layout{Docs: "../docs"}},
		{"docs absolute", state.Layout{Docs: "/docs"}},
		{"decisions traversal", state.Layout{Docs: "docs", Decisions: ptr("../adr")}},
		{"decisions absolute", state.Layout{Docs: "docs", Decisions: ptr("/adr")}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := ResolveLayout(testCase.layout); err == nil {
				t.Fatal("ResolveLayout() error = nil, want an invalid path error")
			}
		})
	}
}
