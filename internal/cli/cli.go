// Package cli implements the atlasctl command facade: global flags, language
// resolution, stable exit codes and the command inventory.
package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/i18n"
)

// Stable exit codes. Scripts and hooks may depend on them.
const (
	// ExitOK means the command completed without errors.
	ExitOK = 0
	// ExitError means the command ran but could not complete.
	ExitError = 1
	// ExitUsage means the invocation was invalid.
	ExitUsage = 2
)

// facadeCommands is the command inventory of the design. It is the single
// source for usage text and unknown-command reports.
var facadeCommands = []string{
	"create",
	"adopt",
	"apply",
	"check",
	"doctor",
	"migrate",
	"configure",
	"profile",
}

// Facade returns the command inventory.
func Facade() []string {
	return append([]string(nil), facadeCommands...)
}

// Env carries the inputs and outputs of a run so the facade is testable
// without touching the process environment or the real streams.
type Env struct {
	Getenv      func(string) string
	Stdout      io.Writer
	Stderr      io.Writer
	Stdin       io.Reader
	Interactive bool
	Project     string
}

// Run executes one invocation and returns its exit code.
func Run(args []string, env Env) int {
	env = withDefaults(env)
	options, usageErr := parse(args)
	catalog := i18n.New(i18n.Resolve(options.lang, env.Getenv))

	if usageErr != nil {
		return reportUsageError(catalog, env.Stderr, usageErr)
	}
	if options.command == "" {
		fmt.Fprintln(env.Stdout, catalog.T(MsgUsage, commandsArgument()))
		if options.help {
			return ExitOK
		}
		return ExitUsage
	}
	if !isFacadeCommand(options.command) {
		fmt.Fprintln(env.Stderr, catalog.T(MsgUnknownCommand, map[string]string{"command": options.command}))
		fmt.Fprintln(env.Stderr, catalog.T(MsgAvailableCommands, commandsArgument()))
		return ExitUsage
	}
	if options.help {
		fmt.Fprintln(env.Stdout, catalog.T(MsgUsage, commandsArgument()))
		return ExitOK
	}

	switch options.command {
	case "create", "adopt", "configure":
		return runSetup(options, env, catalog)
	}

	// Each command arrives in its own change; until then the facade names it
	// instead of pretending to run it.
	fmt.Fprintln(env.Stderr, catalog.T(MsgNotImplemented, map[string]string{"command": options.command}))
	return ExitError
}

func withDefaults(env Env) Env {
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}
	if env.Stdout == nil {
		env.Stdout = os.Stdout
	}
	if env.Stderr == nil {
		env.Stderr = os.Stderr
	}
	if env.Project == "" {
		if project, err := os.Getwd(); err == nil {
			env.Project = project
		}
	}
	return env
}

func reportUsageError(catalog i18n.Catalog, stderr io.Writer, usageErr *usageError) int {
	fmt.Fprintln(stderr, catalog.T(usageErr.id, usageErr.args))
	fmt.Fprintln(stderr, catalog.T(MsgUsage, commandsArgument()))
	return ExitUsage
}

func isFacadeCommand(name string) bool {
	for _, command := range facadeCommands {
		if command == name {
			return true
		}
	}
	return false
}

func commandsArgument() map[string]string {
	return map[string]string{"commands": strings.Join(facadeCommands, ", ")}
}
