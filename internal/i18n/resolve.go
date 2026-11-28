package i18n

import "strings"

// Parse maps a language tag or locale to a supported language. It accepts the
// forms found in flags and environment variables, such as "es", "en_US.UTF-8"
// or "es-MX". The second result is false when the language is not supported.
func Parse(tag string) (Language, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return "", false
	}
	if separator := strings.IndexAny(tag, "_-."); separator >= 0 {
		tag = tag[:separator]
	}

	lang := Language(tag)
	for _, supported := range supportedLanguages {
		if lang == supported {
			return lang, true
		}
	}
	return "", false
}

// Resolve picks the CLI language: the --lang flag wins over ATLAS_LANG, which
// wins over the system locale (LC_ALL and then LANG). English is the last
// resort. Unsupported values are skipped rather than treated as an error.
func Resolve(flag string, getenv func(string) string) Language {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	for _, tag := range []string{flag, getenv("ATLAS_LANG"), getenv("LC_ALL"), getenv("LANG")} {
		if lang, ok := Parse(tag); ok {
			return lang
		}
	}
	return English
}
