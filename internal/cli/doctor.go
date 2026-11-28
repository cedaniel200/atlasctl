package cli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/cedaniel200/atlasctl/internal/commands"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/host"
	"github.com/cedaniel200/atlasctl/internal/i18n"
)

// runDoctor reports the project diagnosis. It exits 0 whenever the diagnosis
// completes, even if it recommends an action; an unreadable state or manifest
// is the only execution error.
func runDoctor(options options, env Env, catalog i18n.Catalog) int {
	explain, usageErr := parseDoctorArgs(options.args)
	if usageErr != nil {
		return reportUsageError(catalog, env.Stderr, usageErr)
	}

	report, err := commands.Doctor(commands.DoctorRequest{
		Project: env.Project,
		Getenv:  env.Getenv,
	})
	if err != nil {
		if errors.Is(err, commands.ErrNotConfigured) {
			fmt.Fprintln(env.Stderr, catalog.T(MsgDoctorNotConfigured, nil))
			return ExitUsage
		}
		fmt.Fprintln(env.Stderr, catalog.T(MsgDoctorFailed, map[string]string{"error": err.Error()}))
		return ExitError
	}

	fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorTitle, nil))
	fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorBinary, map[string]string{
		"version": commands.EngineVersion,
	}))
	fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorState, map[string]string{
		"schema": strconv.Itoa(report.State.SchemaVersion),
		"engine": valueOrNone(catalog, report.State.EngineVersion),
	}))
	manifestLine(catalog, env, report)

	if report.Host.ID == "" {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorHostNone, nil))
	} else {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorHost, map[string]string{
			"id": report.Host.ID, "source": sourceLabel(catalog, report.Host.Source),
		}))
	}
	fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorHostEvidence, map[string]string{
		"evidence": joinedOrNone(catalog, report.Host.Evidence),
	}))
	if report.Resolution.Host == "" {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorDetectNone, map[string]string{
			"reason": reasonLabel(catalog, report.Resolution.Reason),
		}))
	} else {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorDetect, map[string]string{
			"host": report.Resolution.Host, "reason": reasonLabel(catalog, report.Resolution.Reason),
		}))
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorVerification, map[string]string{
			"verification": verificationLabel(catalog, report.ResolutionVerification),
		}))
	}
	fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorDetectEvidence, map[string]string{
		"evidence": joinedOrNone(catalog, detectedHosts(report)),
	}))

	fmt.Fprintln(env.Stdout, doctorHooksLine(catalog, report.Hooks))
	if report.Hooks.HooksPath != "" {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorHooksPath, map[string]string{
			"path": report.Hooks.HooksPath,
		}))
	}
	if report.Spec.Error != "" {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorSpecError, map[string]string{"error": report.Spec.Error}))
	} else if report.Spec.ID == "none" {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorSpecNone, nil))
	} else {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorSpec, map[string]string{
			"id": report.Spec.ID, "root": valueOrNone(catalog, report.Spec.Root),
		}))
	}
	if report.Spec.CLIApplicable {
		if report.Spec.CLIFound {
			fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorSpecCLI, map[string]string{"path": report.Spec.CLIPath}))
		} else {
			fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorSpecCLIMissing, nil))
		}
	}
	if report.LayoutError != "" {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorLayoutError, map[string]string{"error": report.LayoutError}))
	} else {
		fmt.Fprintln(env.Stdout, layoutLine(catalog, report.Layout.Docs, report.Layout.Decisions))
	}
	if report.Setup.Complete {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorSetupComplete, nil))
	} else {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorSetupPending, map[string]string{
			"pending": joinedOrNone(catalog, report.Setup.Pending),
		}))
	}
	if report.Profile == "" {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorProfileNone, nil))
	} else {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorProfile, map[string]string{"profile": report.Profile}))
	}
	fmt.Fprintln(env.Stdout, nextActionLine(catalog, report.NextAction))

	if explain {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorExplainHeader, nil))
		for _, item := range report.Hosts {
			fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorExplainItem, map[string]string{
				"id":           item.ID,
				"signals":      joinedOrNone(catalog, item.Signals),
				"paths":        joinedOrNone(catalog, item.Paths),
				"verification": verificationLabel(catalog, item.Verification),
			}))
		}
	}
	return ExitOK
}

func manifestLine(catalog i18n.Catalog, env Env, report commands.DoctorReport) {
	if report.Manifest == nil {
		fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorManifestMissing, nil))
		return
	}
	generated := catalog.T(MsgCheckNone, nil)
	if !report.Manifest.GeneratedAt.IsZero() {
		generated = report.Manifest.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	fmt.Fprintln(env.Stdout, catalog.T(MsgDoctorManifest, map[string]string{
		"schema":    strconv.Itoa(report.Manifest.SchemaVersion),
		"engine":    valueOrNone(catalog, report.Manifest.EngineVersion),
		"files":     strconv.Itoa(len(report.Manifest.Files)),
		"generated": generated,
	}))
}

func doctorHooksLine(catalog i18n.Catalog, status hooks.Report) string {
	return catalog.T(MsgDoctorHooks, map[string]string{
		"repository": yesNo(catalog, status.Repository),
		"path":       string(status.PathState),
		"dispatcher": yesNo(catalog, status.Dispatcher),
		"guards":     yesNo(catalog, status.GuardsDir),
		"active":     yesNo(catalog, status.Active),
	})
}

func detectedHosts(report commands.DoctorReport) []string {
	hosts := make([]string, 0, len(report.Resolution.Evidence))
	for _, evidence := range report.Resolution.Evidence {
		hosts = append(hosts, evidence.Host)
	}
	return hosts
}

func nextActionLine(catalog i18n.Catalog, action string) string {
	switch action {
	case "configure":
		return catalog.T(MsgDoctorNextConfigure, nil)
	case "apply":
		return catalog.T(MsgDoctorNextApply, nil)
	case "review":
		return catalog.T(MsgDoctorNextReview, nil)
	default:
		return catalog.T(MsgDoctorNextNone, nil)
	}
}

func yesNo(catalog i18n.Catalog, value bool) string {
	if value {
		return catalog.T(MsgCheckYes, nil)
	}
	return catalog.T(MsgCheckNo, nil)
}

func parseDoctorArgs(args []string) (bool, *usageError) {
	explain := false
	for _, arg := range args {
		if arg == "--explain-host" {
			explain = true
			continue
		}
		return false, &usageError{id: MsgUnknownFlag, args: map[string]string{"flag": arg}}
	}
	return explain, nil
}

func sourceLabel(catalog i18n.Catalog, source string) string {
	if source == "" {
		return catalog.T(MsgCheckNone, nil)
	}
	id := "doctor.source." + source
	translated := catalog.T(id, nil)
	if translated == id {
		return source
	}
	return translated
}

func reasonLabel(catalog i18n.Catalog, reason host.Reason) string {
	if reason == "" {
		return catalog.T(MsgCheckNone, nil)
	}
	id := "doctor.reason." + string(reason)
	translated := catalog.T(id, nil)
	if translated == id {
		return string(reason)
	}
	return translated
}

func verificationLabel(catalog i18n.Catalog, verification host.Verification) string {
	id := "doctor.verification." + string(verification)
	translated := catalog.T(id, nil)
	if translated == id {
		return string(verification)
	}
	return translated
}

func valueOrNone(catalog i18n.Catalog, value string) string {
	if value == "" {
		return catalog.T(MsgCheckNone, nil)
	}
	return value
}
