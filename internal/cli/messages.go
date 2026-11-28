package cli

// Message identifiers of the CLI catalog. Callers and tests refer to these
// constants, never to translated text.
const (
	MsgUsage               = "cli.usage"
	MsgUnknownCommand      = "cli.unknown_command"
	MsgAvailableCommands   = "cli.available_commands"
	MsgUnknownFlag         = "cli.unknown_flag"
	MsgMissingFlagValue    = "cli.missing_flag_value"
	MsgConflictingFlags    = "cli.conflicting_flags"
	MsgNotImplemented      = "cli.not_implemented"
	MsgSetupCreated        = "setup.created"
	MsgSetupAdopted        = "setup.adopted"
	MsgSetupConfigured     = "setup.configured"
	MsgSetupAlreadyAdopted = "setup.already_adopted"
	MsgSetupPending        = "setup.pending"
	MsgSetupComplete       = "setup.complete"
	MsgSetupReport         = "setup.report"
	MsgSetupFailed         = "setup.failed"
	MsgApplyDone           = "apply.done"
	MsgApplyDryRun         = "apply.dry_run"
	MsgApplyUpdated        = "apply.updated"
	MsgApplyConserved      = "apply.conserved"
	MsgApplyDivergent      = "apply.divergent"
	MsgApplyPruned         = "apply.pruned"
	MsgApplyNotConfigured  = "apply.not_configured"
	MsgApplyFailed         = "apply.failed"
)
