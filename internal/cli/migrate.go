package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cedaniel200/atlasctl/internal/commands"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/setup"
)

// runMigrate confirms and executes the v1 to v2 migration. It exits 0 for a
// completed migration, an already migrated project or a cancelled one; 1 when
// the migration cannot proceed and 2 for usage or a missing --yes.
func runMigrate(options options, env Env, catalog i18n.Catalog) int {
	yes, usageErr := parseMigrateArgs(options.args)
	if usageErr != nil {
		return reportUsageError(catalog, env.Stderr, usageErr)
	}
	interactive := env.Interactive && env.Stdin != nil
	terminal := setup.NewTerminal(env.Stdin, env.Stdout)

	confirm := func(plan commands.MigratePlan) (bool, error) {
		printMigratePlan(catalog, env, plan)
		if yes {
			return true, nil
		}
		if !interactive {
			return false, commands.ErrConfirmationRequired
		}
		answer, err := terminal.Ask(catalog.T(MsgMigrateConfirm, nil), "n")
		if err != nil {
			return false, err
		}
		return isAffirmative(answer), nil
	}

	result, err := commands.Migrate(commands.MigrateRequest{
		Project: env.Project,
		Confirm: confirm,
	})
	if err != nil {
		switch {
		case errors.Is(err, commands.ErrConfirmationRequired):
			fmt.Fprintln(env.Stderr, catalog.T(MsgMigrateConfirmationRequired, nil))
			return ExitUsage
		case errors.Is(err, commands.ErrMigrationCancelled):
			fmt.Fprintln(env.Stdout, catalog.T(MsgMigrateCancelled, nil))
			return ExitOK
		default:
			fmt.Fprintln(env.Stderr, catalog.T(MsgMigrateFailed, map[string]string{"error": err.Error()}))
			return ExitError
		}
	}
	if result.Already {
		fmt.Fprintln(env.Stdout, catalog.T(MsgMigrateAlready, nil))
		return ExitOK
	}

	fmt.Fprintln(env.Stdout, catalog.T(MsgMigrateDone, map[string]string{"backup": result.BackupDir}))
	fmt.Fprintln(env.Stdout, catalog.T(MsgMigrateDeferred, nil))
	return ExitOK
}

func printMigratePlan(catalog i18n.Catalog, env Env, plan commands.MigratePlan) {
	fmt.Fprintln(env.Stdout, catalog.T(MsgMigratePlan, map[string]string{
		"files": strconv.Itoa(plan.Files),
	}))
}

func parseMigrateArgs(args []string) (bool, *usageError) {
	yes := false
	for _, arg := range args {
		if arg != "--yes" {
			return false, &usageError{id: MsgUnknownFlag, args: map[string]string{"flag": arg}}
		}
		yes = true
	}
	return yes, nil
}

func isAffirmative(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes", "s", "si", "sí":
		return true
	default:
		return false
	}
}
