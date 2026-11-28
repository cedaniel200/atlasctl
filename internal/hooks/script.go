package hooks

import (
	_ "embed"
	"strings"
)

//go:embed pre-commit.sh
var dispatcherTemplate string

const specRootPlaceholder = "{{SPEC_ENGINE_ROOT}}"

// Script returns the generated dispatcher for specRoot. The plan publishes it
// as a generated artifact; the applier writes it with the executable mode.
func Script(specRoot string) []byte {
	return []byte(strings.ReplaceAll(dispatcherTemplate, specRootPlaceholder, shellQuote(specRoot)))
}

// shellQuote returns a single-quoted POSIX shell word. The spec engine root
// comes from project state and may contain spaces or quotes; embedding it
// unquoted would turn it into shell syntax.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
