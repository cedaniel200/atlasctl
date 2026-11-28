package i18n

import "testing"

func environment(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func TestParseLanguage(t *testing.T) {
	cases := []struct {
		tag  string
		want Language
		ok   bool
	}{
		{"es", Spanish, true},
		{"ES", Spanish, true},
		{"es_ES.UTF-8", Spanish, true},
		{"es-MX", Spanish, true},
		{" en_US.UTF-8 ", English, true},
		{"en", English, true},
		{"fr", "", false},
		{"C", "", false},
		{"POSIX", "", false},
		{"", "", false},
		{"   ", "", false},
	}
	for _, testCase := range cases {
		got, ok := Parse(testCase.tag)
		if got != testCase.want || ok != testCase.ok {
			t.Errorf("Parse(%q) = (%q, %t), want (%q, %t)", testCase.tag, got, ok, testCase.want, testCase.ok)
		}
	}
}

func TestResolvePrefersTheFlag(t *testing.T) {
	env := environment(map[string]string{
		"ATLAS_LANG": "en",
		"LC_ALL":     "en_US.UTF-8",
		"LANG":       "en_US.UTF-8",
	})
	if got := Resolve("es", env); got != Spanish {
		t.Errorf("Resolve() = %q, want %q", got, Spanish)
	}
}

func TestResolveUsesAtlasLangBeforeTheLocale(t *testing.T) {
	env := environment(map[string]string{
		"ATLAS_LANG": "es",
		"LC_ALL":     "en_US.UTF-8",
		"LANG":       "en_US.UTF-8",
	})
	if got := Resolve("", env); got != Spanish {
		t.Errorf("Resolve() = %q, want %q", got, Spanish)
	}
}

func TestResolveUsesLCAllBeforeLang(t *testing.T) {
	env := environment(map[string]string{
		"LC_ALL": "es_ES.UTF-8",
		"LANG":   "en_US.UTF-8",
	})
	if got := Resolve("", env); got != Spanish {
		t.Errorf("Resolve() = %q, want %q", got, Spanish)
	}
}

func TestResolveUsesLangAsLastLocaleSource(t *testing.T) {
	env := environment(map[string]string{"LANG": "es_ES.UTF-8"})
	if got := Resolve("", env); got != Spanish {
		t.Errorf("Resolve() = %q, want %q", got, Spanish)
	}
}

func TestResolveSkipsUnsupportedValues(t *testing.T) {
	env := environment(map[string]string{
		"ATLAS_LANG": "fr",
		"LC_ALL":     "es_ES.UTF-8",
	})
	if got := Resolve("de", env); got != Spanish {
		t.Errorf("Resolve() = %q, want %q", got, Spanish)
	}
}

func TestResolveFallsBackToEnglish(t *testing.T) {
	if got := Resolve("", nil); got != English {
		t.Errorf("Resolve() = %q, want %q", got, English)
	}
	env := environment(map[string]string{"LANG": "C"})
	if got := Resolve("fr", env); got != English {
		t.Errorf("Resolve() = %q, want %q", got, English)
	}
}
