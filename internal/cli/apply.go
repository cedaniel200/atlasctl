package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/commands"
	"github.com/cedaniel200/atlasctl/internal/i18n"
)

// runApply updates an adopted project with the existing state and manifest. It
// never asks setup questions: flags only override bundles, host or pruning.
func runApply(options options, env Env, catalog i18n.Catalog) int {
	request, usageErr := parseApplyArgs(options.args)
	if usageErr != nil {
		return reportUsageError(catalog, env.Stderr, usageErr)
	}
	request.Project = env.Project

	result, err := commands.Apply(request)
	if err != nil {
		if errors.Is(err, commands.ErrNotConfigured) {
			fmt.Fprintln(env.Stderr, catalog.T(MsgApplyNotConfigured, nil))
			return ExitUsage
		}
		fmt.Fprintln(env.Stderr, catalog.T(MsgApplyFailed, map[string]string{"error": err.Error()}))
		return ExitError
	}

	if result.DryRun {
		fmt.Fprintln(env.Stdout, catalog.T(MsgApplyDryRun, nil))
	} else {
		fmt.Fprintln(env.Stdout, catalog.T(MsgApplyDone, nil))
	}
	if result.Report != nil {
		report := result.Report
		fmt.Fprintln(env.Stdout, catalog.T(MsgApplyUpdated, map[string]string{"count": strconv.Itoa(len(report.Updated))}))
		fmt.Fprintln(env.Stdout, catalog.T(MsgApplyConserved, map[string]string{"count": strconv.Itoa(len(report.Conserved))}))
		fmt.Fprintln(env.Stdout, catalog.T(MsgApplyDivergent, map[string]string{"count": strconv.Itoa(len(report.Divergent))}))
		fmt.Fprintln(env.Stdout, catalog.T(MsgApplyPruned, map[string]string{"count": strconv.Itoa(len(report.Pruned))}))
	}
	return ExitOK
}

// parseApplyArgs parses the flags that may override the stored configuration.
func parseApplyArgs(args []string) (commands.ApplyRequest, *usageError) {
	var request commands.ApplyRequest

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--prune":
			request.Prune = true
		case arg == "--dry-run":
			request.DryRun = true
		case matchesFlag(arg, "--host"):
			value, next, err := flagValue(args, index, "--host")
			if err != nil {
				return request, err
			}
			request.Host, index = value, next
		case matchesFlag(arg, "--snapshot"):
			value, next, err := flagValue(args, index, "--snapshot")
			if err != nil {
				return request, err
			}
			request.Snapshot, index = value, next
		case matchesFlag(arg, "--preset"):
			value, next, err := flagValue(args, index, "--preset")
			if err != nil {
				return request, err
			}
			request.Preset, index = value, next
		case matchesFlag(arg, "--bundle"):
			value, next, err := flagValue(args, index, "--bundle")
			if err != nil {
				return request, err
			}
			for _, item := range strings.Split(value, ",") {
				if item = strings.TrimSpace(item); item != "" {
					request.Bundles = append(request.Bundles, item)
				}
			}
			index = next
		default:
			return request, &usageError{id: MsgUnknownFlag, args: map[string]string{"flag": arg}}
		}
	}
	return request, nil
}
