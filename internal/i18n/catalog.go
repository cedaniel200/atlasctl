// Package i18n holds the CLI message catalog. Messages are identified by a
// stable id and may carry named markers such as {command}; text is never built
// by concatenating sentences.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed messages/*.json
var messageFiles embed.FS

// Language identifies a catalog. Adding a language is adding a message file.
type Language string

const (
	Spanish Language = "es"
	English Language = "en"
)

var supportedLanguages = []Language{Spanish, English}

// Catalog resolves a message id to a rendered message.
type Catalog interface {
	T(id string, args map[string]string) string
}

// New returns the catalog for lang. An unsupported language falls back to
// english, the language of last resort.
func New(lang Language) Catalog {
	if _, ok := translations[lang]; !ok {
		lang = English
	}
	return &catalog{
		messages: translations[lang],
		fallback: translations[English],
	}
}

type catalog struct {
	messages map[string]string
	fallback map[string]string
}

func (c *catalog) T(id string, args map[string]string) string {
	template, ok := c.messages[id]
	if !ok {
		template, ok = c.fallback[id]
	}
	if !ok {
		return id
	}
	if len(args) == 0 {
		return template
	}

	replacements := make([]string, 0, len(args)*2)
	for name, value := range args {
		replacements = append(replacements, "{"+name+"}", value)
	}
	return strings.NewReplacer(replacements...).Replace(template)
}

var translations = mustLoadCatalogs()

// mustLoadCatalogs parses the embedded catalogs. A failure here means the
// binary was built with a broken message file, not that runtime input is
// invalid, so it panics at startup.
func mustLoadCatalogs() map[Language]map[string]string {
	catalogs := make(map[Language]map[string]string, len(supportedLanguages))
	for _, lang := range supportedLanguages {
		path := "messages/" + string(lang) + ".json"
		data, err := messageFiles.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("i18n: read %s: %v", path, err))
		}
		messages := map[string]string{}
		if err := json.Unmarshal(data, &messages); err != nil {
			panic(fmt.Sprintf("i18n: decode %s: %v", path, err))
		}
		catalogs[lang] = messages
	}
	return catalogs
}
