package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/commands"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/setup"
)

func runSetup(options options, env Env, catalog i18n.Catalog) int {
	answers, snapshot, usageErr := parseSetupArgs(options.args)
	if usageErr != nil {
		return reportUsageError(catalog, env.Stderr, usageErr)
	}

	interactive := env.Interactive && env.Stdin != nil
	var prompter setup.Prompter
	if interactive {
		prompter = setup.NewTerminal(env.Stdin, env.Stdout)
	}
	language := i18n.Resolve(options.lang, env.Getenv)

	request := commands.Request{
		Project:     env.Project,
		Answers:     answers,
		Snapshot:    snapshot,
		Interactive: interactive,
		Prompter:    prompter,
		Getenv:      env.Getenv,
		Catalog:     catalog,
		CLILanguage: string(language),
		LangSet:     options.lang != "",
	}

	var result commands.Result
	var err error
	switch options.command {
	case "create":
		result, err = commands.Create(request)
	case "adopt":
		result, err = commands.Adopt(request)
	case "configure":
		result, err = commands.Configure(request)
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, catalog.T(MsgSetupFailed, map[string]string{"error": err.Error()}))
		return ExitError
	}
	if result.AlreadyAdopted {
		fmt.Fprintln(env.Stdout, catalog.T(MsgSetupAlreadyAdopted, nil))
		return ExitOK
	}
	switch options.command {
	case "create":
		fmt.Fprintln(env.Stdout, catalog.T(MsgSetupCreated, nil))
	case "adopt":
		fmt.Fprintln(env.Stdout, catalog.T(MsgSetupAdopted, nil))
	case "configure":
		fmt.Fprintln(env.Stdout, catalog.T(MsgSetupConfigured, nil))
	}
	if result.Report != nil {
		fmt.Fprintln(env.Stdout, catalog.T(MsgSetupReport, map[string]string{
			"updated":   strconv.Itoa(len(result.Report.Updated)),
			"conserved": strconv.Itoa(len(result.Report.Conserved)),
			"divergent": strconv.Itoa(len(result.Report.Divergent)),
			"pruned":    strconv.Itoa(len(result.Report.Pruned)),
		}))
	}
	if len(result.Pending) > 0 {
		fmt.Fprintln(env.Stdout, catalog.T(MsgSetupPending, map[string]string{"pending": strings.Join(result.Pending, ", ")}))
	} else {
		fmt.Fprintln(env.Stdout, catalog.T(MsgSetupComplete, nil))
	}
	return ExitOK
}

// parseSetupArgs parses the flag equivalents of the interview.
func parseSetupArgs(args []string) (setup.Answers, string, *usageError) {
	var answers setup.Answers
	var snapshot string

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--no-docs":
			answers.NoDocs = true
		case arg == "--no-decisions":
			answers.NoDecisions = true
		case matchesFlag(arg, "--host"):
			value, next, err := flagValue(args, index, "--host")
			if err != nil {
				return answers, snapshot, err
			}
			answers.Host, index = value, next
		case matchesFlag(arg, "--docs"):
			value, next, err := flagValue(args, index, "--docs")
			if err != nil {
				return answers, snapshot, err
			}
			answers.Docs, index = value, next
		case matchesFlag(arg, "--decisions"):
			value, next, err := flagValue(args, index, "--decisions")
			if err != nil {
				return answers, snapshot, err
			}
			answers.Decisions, index = value, next
		case matchesFlag(arg, "--bundle"):
			value, next, err := flagValue(args, index, "--bundle")
			if err != nil {
				return answers, snapshot, err
			}
			for _, item := range strings.Split(value, ",") {
				if item = strings.TrimSpace(item); item != "" {
					answers.Bundles = append(answers.Bundles, item)
				}
			}
			index = next
		case matchesFlag(arg, "--preset"):
			value, next, err := flagValue(args, index, "--preset")
			if err != nil {
				return answers, snapshot, err
			}
			answers.Preset, index = value, next
		case matchesFlag(arg, "--profile"):
			value, next, err := flagValue(args, index, "--profile")
			if err != nil {
				return answers, snapshot, err
			}
			answers.Profile, index = value, next
		case matchesFlag(arg, "--snapshot"):
			value, next, err := flagValue(args, index, "--snapshot")
			if err != nil {
				return answers, snapshot, err
			}
			snapshot, index = value, next
		default:
			return answers, snapshot, &usageError{id: MsgUnknownFlag, args: map[string]string{"flag": arg}}
		}
	}

	if answers.Docs != "" && answers.NoDocs {
		return answers, snapshot, &usageError{id: MsgConflictingFlags, args: map[string]string{
			"first": "--docs", "second": "--no-docs",
		}}
	}
	if answers.Decisions != "" && answers.NoDecisions {
		return answers, snapshot, &usageError{id: MsgConflictingFlags, args: map[string]string{
			"first": "--decisions", "second": "--no-decisions",
		}}
	}
	return answers, snapshot, nil
}

func matchesFlag(arg, name string) bool {
	return arg == name || strings.HasPrefix(arg, name+"=")
}

func flagValue(args []string, index int, name string) (string, int, *usageError) {
	arg := args[index]
	if strings.HasPrefix(arg, name+"=") {
		value := strings.TrimPrefix(arg, name+"=")
		if value == "" {
			return "", index, &usageError{id: MsgMissingFlagValue, args: map[string]string{"flag": name}}
		}
		return value, index, nil
	}
	if index+1 >= len(args) {
		return "", index, &usageError{id: MsgMissingFlagValue, args: map[string]string{"flag": name}}
	}
	return args[index+1], index + 1, nil
}
