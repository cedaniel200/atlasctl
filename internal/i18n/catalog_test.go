package i18n

import (
	"strings"
	"testing"
)

func TestCatalogHasSameIDsInEveryLanguage(t *testing.T) {
	spanish, english := translations[Spanish], translations[English]
	if len(spanish) == 0 {
		t.Fatal("spanish catalog is empty")
	}
	if len(english) == 0 {
		t.Fatal("english catalog is empty")
	}

	for id, message := range spanish {
		if strings.TrimSpace(message) == "" {
			t.Errorf("message %q is empty in %s", id, Spanish)
		}
		other, ok := english[id]
		if !ok {
			t.Errorf("message %q is missing in %s", id, English)
			continue
		}
		if strings.TrimSpace(other) == "" {
			t.Errorf("message %q is empty in %s", id, English)
		}
	}
	for id := range english {
		if _, ok := spanish[id]; !ok {
			t.Errorf("message %q is missing in %s", id, Spanish)
		}
	}
}

func TestTReplacesNamedMarkers(t *testing.T) {
	for _, lang := range supportedLanguages {
		got := New(lang).T("cli.unknown_command", map[string]string{"command": "frobnicate"})
		if !strings.Contains(got, "frobnicate") {
			t.Errorf("%s message %q does not contain the marker value", lang, got)
		}
		if strings.Contains(got, "{command}") {
			t.Errorf("%s message %q kept the unresolved marker", lang, got)
		}
	}
}

func TestTWithoutArgumentsReturnsTheTemplate(t *testing.T) {
	got := New(English).T("cli.unknown_command", nil)
	if got == "cli.unknown_command" {
		t.Fatal("message not found in the catalog")
	}
	if !strings.Contains(got, "{command}") {
		t.Errorf("message %q does not keep its marker when no arguments are given", got)
	}
}

func TestTUnknownIDReturnsTheID(t *testing.T) {
	const id = "cli.does_not_exist"
	if got := New(Spanish).T(id, map[string]string{"x": "y"}); got != id {
		t.Errorf("T(%q) = %q, want the id itself", id, got)
	}
}

func TestNewFallsBackToEnglish(t *testing.T) {
	got := New(Language("fr")).T("cli.unknown_command", map[string]string{"command": "x"})
	want := New(English).T("cli.unknown_command", map[string]string{"command": "x"})
	if got != want {
		t.Errorf("T() = %q, want the english catalog %q", got, want)
	}
}
