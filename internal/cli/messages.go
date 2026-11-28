package cli

// Message identifiers of the CLI catalog. Callers and tests refer to these
// constants, never to translated text.
const (
	MsgUsage             = "cli.usage"
	MsgUnknownCommand    = "cli.unknown_command"
	MsgAvailableCommands = "cli.available_commands"
	MsgUnknownFlag       = "cli.unknown_flag"
	MsgMissingFlagValue  = "cli.missing_flag_value"
	MsgConflictingFlags  = "cli.conflicting_flags"
	MsgNotImplemented    = "cli.not_implemented"
	MsgSetupCreated      = "setup.created"
	MsgSetupPending      = "setup.pending"
	MsgSetupComplete     = "setup.complete"
	MsgSetupReport       = "setup.report"
	MsgSetupFailed       = "setup.failed"
)
