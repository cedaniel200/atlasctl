package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cedaniel200/atlasctl/internal/i18n"
)

func run(args []string, env map[string]string) (stdout, stderr string, code int) {
	var out, errOut bytes.Buffer
	code = Run(args, Env{
		Getenv: func(key string) string { return env[key] },
		Stdout: &out,
		Stderr: &errOut,
	})
	return out.String(), errOut.String(), code
}

func commandsMarker() string {
	return strings.Join(Facade(), ", ")
}

func message(lang i18n.Language, id string, args map[string]string) string {
	return i18n.New(lang).T(id, args) + "\n"
}

func TestExitCodesAreStable(t *testing.T) {
	if ExitOK != 0 || ExitError != 1 || ExitUsage != 2 {
		t.Errorf("exit codes = (%d, %d, %d), want (0, 1, 2)", ExitOK, ExitError, ExitUsage)
	}
}

func TestFacadeListsTheEightDesignCommands(t *testing.T) {
	want := []string{"create", "adopt", "apply", "check", "doctor", "migrate", "configure", "profile"}
	got := Facade()
	if len(got) != len(want) {
		t.Fatalf("Facade() = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("Facade()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func TestUnknownCommandFailsWithUsageAndListsTheFacade(t *testing.T) {
	stdout, stderr, code := run([]string{"publish"}, nil)

	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	want := message(i18n.English, MsgUnknownCommand, map[string]string{"command": "publish"}) +
		message(i18n.English, MsgAvailableCommands, map[string]string{"commands": commandsMarker()})
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	for _, command := range Facade() {
		if !strings.Contains(stderr, command) {
			t.Errorf("stderr does not list command %q", command)
		}
	}
}

func TestUnknownCommandUsesTheResolvedLanguage(t *testing.T) {
	_, english, _ := run([]string{"publish"}, nil)
	_, spanish, code := run([]string{"--lang", "es", "publish"}, nil)

	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	want := message(i18n.Spanish, MsgUnknownCommand, map[string]string{"command": "publish"}) +
		message(i18n.Spanish, MsgAvailableCommands, map[string]string{"commands": commandsMarker()})
	if spanish != want {
		t.Errorf("stderr = %q, want %q", spanish, want)
	}
	if spanish == english {
		t.Error("the spanish and english catalogs produced the same message")
	}
}

func TestNoArgumentsPrintsUsageAndFails(t *testing.T) {
	stdout, stderr, code := run(nil, nil)

	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if want := message(i18n.English, MsgUsage, map[string]string{"commands": commandsMarker()}); stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestHelpPrintsUsageAndSucceeds(t *testing.T) {
	stdout, stderr, code := run([]string{"--help"}, nil)

	if code != ExitOK {
		t.Errorf("exit code = %d, want %d", code, ExitOK)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if want := message(i18n.English, MsgUsage, map[string]string{"commands": commandsMarker()}); stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestKnownCommandIsNotImplementedYet(t *testing.T) {
	stdout, stderr, code := run([]string{"profile"}, nil)

	if code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if want := message(i18n.English, MsgNotImplemented, map[string]string{"command": "profile"}); stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestUnknownFlagFailsWithUsage(t *testing.T) {
	stdout, stderr, code := run([]string{"--nope"}, nil)

	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	want := message(i18n.English, MsgUnknownFlag, map[string]string{"flag": "--nope"}) +
		message(i18n.English, MsgUsage, map[string]string{"commands": commandsMarker()})
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}

func TestLanguageFlagWithoutValueFailsWithUsage(t *testing.T) {
	_, stderr, code := run([]string{"--lang"}, nil)

	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	want := message(i18n.English, MsgMissingFlagValue, map[string]string{"flag": "--lang"}) +
		message(i18n.English, MsgUsage, map[string]string{"commands": commandsMarker()})
	if stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
}
