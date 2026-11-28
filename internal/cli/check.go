package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/classify"
	"github.com/cedaniel200/atlasctl/internal/commands"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func runCheck(options options, env Env, catalog i18n.Catalog) int {
	if len(options.args) > 0 {
		return reportUsageError(catalog, env.Stderr, &usageError{
			id: MsgUnknownFlag, args: map[string]string{"flag": options.args[0]},
		})
	}

	report, err := commands.Check(commands.CheckRequest{Project: env.Project})
	if err != nil {
		if errors.Is(err, commands.ErrNotConfigured) {
			fmt.Fprintln(env.Stderr, catalog.T(MsgCheckNotConfigured, nil))
			return ExitUsage
		}
		fmt.Fprintln(env.Stderr, catalog.T(MsgCheckFailed, map[string]string{"error": err.Error()}))
		return ExitError
	}

	fmt.Fprintln(env.Stdout, catalog.T(MsgCheckTitle, nil))
	for _, item := range report.Setup {
		fmt.Fprintln(env.Stdout, setupLine(catalog, item))
	}
	if report.Manifest == nil {
		fmt.Fprintln(env.Stdout, catalog.T(MsgCheckManifestMissing, nil))
	} else {
		fmt.Fprintln(env.Stdout, catalog.T(MsgCheckManifest, map[string]string{
			"files": strconv.Itoa(len(report.Manifest.Files)),
		}))
	}
	for _, file := range report.Files {
		fmt.Fprintln(env.Stdout, fileLine(catalog, file))
	}
	fmt.Fprintln(env.Stdout, dispatcherLine(catalog, report.Dispatcher))
	fmt.Fprintln(env.Stdout, hooksLine(catalog, report.Hooks))
	fmt.Fprintln(env.Stdout, specLine(catalog, report.Spec))
	fmt.Fprintln(env.Stdout, layoutLine(catalog, report.Layout.Docs, report.Layout.Decisions))

	if report.Recommendations == 0 {
		fmt.Fprintln(env.Stdout, catalog.T(MsgCheckSummaryOK, nil))
		return ExitOK
	}
	fmt.Fprintln(env.Stdout, catalog.T(MsgCheckSummaryFindings, map[string]string{
		"count": strconv.Itoa(report.Recommendations),
	}))
	return ExitFindings
}

func setupLine(catalog i18n.Catalog, item commands.SetupStatus) string {
	value := item.Value
	if value == "" {
		value = catalog.T(MsgCheckNone, nil)
	}
	status := MsgCheckStateSet
	switch {
	case item.Pending:
		status = MsgCheckStatePending
	case item.ID == commands.SetupProfile:
		if item.Value == "" {
			status = MsgCheckStateNotDetected
		} else {
			status = MsgCheckStateDetected
		}
	case item.Value == "":
		status = MsgCheckStateAbsent
	}
	return catalog.T(MsgCheckSetupItem, map[string]string{
		"id": item.ID, "value": value, "status": catalog.T(status, nil),
	})
}

func fileLine(catalog i18n.Catalog, file commands.FileStatus) string {
	status := MsgCheckFileGeneratedModified
	switch {
	case file.Layer == state.LayerBase && file.Status == classify.StatusModified:
		status = MsgCheckFileBaseModified
	case file.Layer == state.LayerBase && file.Status == classify.StatusAbsent:
		status = MsgCheckFileBaseMissing
	case file.Status == classify.StatusAbsent:
		status = MsgCheckFileGeneratedMissing
	}
	return catalog.T(MsgCheckFile, map[string]string{
		"path": file.Path, "layer": string(file.Layer), "status": catalog.T(status, nil),
	})
}

func dispatcherLine(catalog i18n.Catalog, dispatcher *commands.FileStatus) string {
	switch {
	case dispatcher == nil:
		return catalog.T(MsgCheckDispatcherMissing, nil)
	case !dispatcher.Tracked:
		return catalog.T(MsgCheckDispatcherUntracked, nil)
	case dispatcher.Status == classify.StatusAbsent:
		return catalog.T(MsgCheckDispatcherMissing, nil)
	case dispatcher.Status == classify.StatusModified:
		return catalog.T(MsgCheckDispatcherModified, nil)
	case !dispatcher.Executable:
		return catalog.T(MsgCheckDispatcherPermission, nil)
	default:
		return catalog.T(MsgCheckDispatcherOK, nil)
	}
}

func hooksLine(catalog i18n.Catalog, status hooks.Report) string {
	yes := catalog.T(MsgCheckYes, nil)
	no := catalog.T(MsgCheckNo, nil)
	switch {
	case !status.Repository:
		return catalog.T(MsgCheckHooksNoRepo, nil)
	case status.PathState == hooks.PathForeign:
		return catalog.T(MsgCheckHooksForeign, map[string]string{"path": status.HooksPath})
	case status.Active && status.GuardsDir:
		return catalog.T(MsgCheckHooksActive, map[string]string{
			"guards": strconv.Itoa(len(status.Guards)),
		})
	default:
		dispatcher, guards := no, no
		if status.Dispatcher {
			dispatcher = yes
		}
		if status.GuardsDir {
			guards = yes
		}
		return catalog.T(MsgCheckHooksInactive, map[string]string{
			"path": string(status.PathState), "dispatcher": dispatcher, "guards": guards,
		})
	}
}

func specLine(catalog i18n.Catalog, spec commands.SpecStatus) string {
	switch {
	case spec.ID == "none":
		return catalog.T(MsgCheckSpecNone, nil)
	case spec.Root == "":
		return catalog.T(MsgCheckSpecID, map[string]string{"id": spec.ID})
	default:
		return catalog.T(MsgCheckSpec, map[string]string{"id": spec.ID, "root": spec.Root})
	}
}

func layoutLine(catalog i18n.Catalog, docs, decisions string) string {
	if docs == "" {
		docs = catalog.T(MsgCheckNone, nil)
	}
	if decisions == "" {
		decisions = catalog.T(MsgCheckNone, nil)
	}
	return catalog.T(MsgCheckLayout, map[string]string{"docs": docs, "decisions": decisions})
}

func joinedOrNone(catalog i18n.Catalog, values []string) string {
	if len(values) == 0 {
		return catalog.T(MsgCheckNone, nil)
	}
	return strings.Join(values, ", ")
}
