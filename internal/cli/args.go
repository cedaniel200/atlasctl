package cli

import "strings"

// options is the result of parsing the global flags that precede the command.
type options struct {
	lang    string
	command string
	help    bool
}

// usageError carries a message id and its named markers; the renderer decides
// the language.
type usageError struct {
	id   string
	args map[string]string
}

func parse(args []string) (options, *usageError) {
	var parsed options
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-h" || arg == "--help":
			parsed.help = true
		case arg == "--lang":
			if index+1 >= len(args) {
				return parsed, missingLanguageValue()
			}
			parsed.lang = args[index+1]
			index++
		case strings.HasPrefix(arg, "--lang="):
			parsed.lang = strings.TrimPrefix(arg, "--lang=")
			if parsed.lang == "" {
				return parsed, missingLanguageValue()
			}
		case strings.HasPrefix(arg, "-"):
			return parsed, &usageError{id: MsgUnknownFlag, args: map[string]string{"flag": arg}}
		default:
			parsed.command = arg
			return parsed, nil
		}
	}
	return parsed, nil
}

func missingLanguageValue() *usageError {
	return &usageError{id: MsgMissingFlagValue, args: map[string]string{"flag": "--lang"}}
}
