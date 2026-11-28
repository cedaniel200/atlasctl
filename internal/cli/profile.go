package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/commands"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/setup"
)

func runProfile(options options, env Env, catalog i18n.Catalog) int {
	if len(options.args) == 0 || options.args[0] != "detect" {
		fmt.Fprintln(env.Stderr, catalog.T(MsgProfileUsage, nil))
		return ExitUsage
	}
	write, force, usageErr := parseProfileArgs(options.args[1:])
	if usageErr != nil {
		return reportUsageError(catalog, env.Stderr, usageErr)
	}

	detection := commands.DetectProfile(env.Project)
	if !detection.Found {
		fmt.Fprintln(env.Stdout, catalog.T(MsgProfileNone, nil))
		return ExitOK
	}
	printProfileProposal(catalog, env, detection)
	exists := commands.ProfileExists(env.Project, detection.Proposal.ID)
	interactive := env.Interactive && env.Stdin != nil
	terminal := setup.NewTerminal(env.Stdin, env.Stdout)

	shouldWrite := write || force
	if !write && !force {
		if interactive {
			question := MsgProfileWritePrompt
			if exists {
				question = MsgProfileOverwritePrompt
			}
			answer, err := terminal.Ask(catalog.T(question, map[string]string{"path": detection.Path}), "n")
			if err != nil {
				fmt.Fprintln(env.Stderr, catalog.T(MsgProfileFailed, map[string]string{"error": err.Error()}))
				return ExitError
			}
			shouldWrite = isAffirmative(answer)
			if shouldWrite && exists {
				force = true
			}
		} else {
			if exists {
				fmt.Fprintln(env.Stdout, catalog.T(MsgProfileExists, map[string]string{"path": detection.Path}))
			}
			fmt.Fprintln(env.Stdout, catalog.T(MsgProfileProposalOnly, nil))
			return ExitOK
		}
	}

	if shouldWrite && exists && !force {
		if interactive {
			answer, err := terminal.Ask(catalog.T(MsgProfileOverwritePrompt, map[string]string{"path": detection.Path}), "n")
			if err != nil {
				fmt.Fprintln(env.Stderr, catalog.T(MsgProfileFailed, map[string]string{"error": err.Error()}))
				return ExitError
			}
			if !isAffirmative(answer) {
				fmt.Fprintln(env.Stdout, catalog.T(MsgProfileProposalOnly, nil))
				return ExitOK
			}
			force = true
		} else {
			fmt.Fprintln(env.Stdout, catalog.T(MsgProfileExists, map[string]string{"path": detection.Path}))
			return ExitUsage
		}
	}
	if !shouldWrite {
		fmt.Fprintln(env.Stdout, catalog.T(MsgProfileProposalOnly, nil))
		return ExitOK
	}

	result, err := commands.WriteProfile(commands.ProfileWriteRequest{
		Project:  env.Project,
		Proposal: detection.Proposal,
		Force:    force,
	})
	if err != nil {
		switch {
		case errors.Is(err, commands.ErrProfileExists):
			fmt.Fprintln(env.Stdout, catalog.T(MsgProfileExists, map[string]string{"path": detection.Path}))
			return ExitUsage
		case errors.Is(err, commands.ErrNotConfigured):
			fmt.Fprintln(env.Stderr, catalog.T(MsgProfileNotConfigured, nil))
			return ExitUsage
		default:
			fmt.Fprintln(env.Stderr, catalog.T(MsgProfileFailed, map[string]string{"error": err.Error()}))
			return ExitError
		}
	}
	fmt.Fprintln(env.Stdout, catalog.T(MsgProfileWritten, map[string]string{
		"path": result.Path, "id": result.Profile,
	}))
	return ExitOK
}

func printProfileProposal(catalog i18n.Catalog, env Env, detection commands.ProfileDetection) {
	proposal := detection.Proposal
	fmt.Fprintln(env.Stdout, catalog.T(MsgProfileDetected, map[string]string{"id": proposal.ID}))
	fmt.Fprintln(env.Stdout, catalog.T(MsgProfileEvidence, map[string]string{
		"evidence": strings.Join(proposal.Evidence, ", "),
	}))
	fmt.Fprintln(env.Stdout, catalog.T(MsgProfileDerived, map[string]string{
		"language":        proposal.Language,
		"runtime":         proposal.Runtime,
		"package_manager": proposal.PackageManager,
	}))
	fmt.Fprintln(env.Stdout, catalog.T(MsgProfileStructure, map[string]string{
		"source": proposal.Source, "tests": proposal.Tests,
	}))
	fmt.Fprintln(env.Stdout, catalog.T(MsgProfileCommandsTBD, nil))
}

func parseProfileArgs(args []string) (bool, bool, *usageError) {
	write := false
	force := false
	for _, arg := range args {
		switch arg {
		case "--write":
			write = true
		case "--force":
			force = true
		default:
			return false, false, &usageError{id: MsgUnknownFlag, args: map[string]string{"flag": arg}}
		}
	}
	return write, force, nil
}
