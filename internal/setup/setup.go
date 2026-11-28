// Package setup resolves the answers of the initial configuration: host,
// language, project layout, profile and bundles. It detects first, asks only
// what remains ambiguous and records unanswered questions as pending. It never
// writes to the project.
package setup

import (
	"errors"
	"fmt"

	"github.com/cedaniel200/atlasctl/internal/host"
	"github.com/cedaniel200/atlasctl/internal/i18n"
	"github.com/cedaniel200/atlasctl/internal/spec"
	"github.com/cedaniel200/atlasctl/internal/state"
)

// DefaultBundle is the activation set of a new project in 0.6.0.
const DefaultBundle = "dev"

// Answers carries the flag equivalents of the interview. Empty values mean the
// question was not answered by a flag.
type Answers struct {
	Host        string
	Docs        string
	NoDocs      bool
	Decisions   string
	NoDecisions bool
	Bundles     []string
	Preset      string
	Profile     string
}

// Request is the input of a setup resolution.
type Request struct {
	Project     string
	Existing    *state.State
	Answers     Answers
	Interactive bool
	Prompter    Prompter
	Catalog     i18n.Catalog
	Getenv      func(string) string
	CLILanguage string
	LangSet     bool
}

// Result is the resolved state fragment plus the active bundle request. The
// command layer completes the state with the engine version, content version
// and hook status.
type Result struct {
	State    state.State
	Pending  []string
	Complete bool
	Bundles  []string
	Preset   string
	Strategy host.Strategy
}

// Resolve detects, asks and leaves pending questions without writing.
func Resolve(request Request) (Result, error) {
	if request.Project == "" {
		return Result{}, errors.New("setup: project root is required")
	}
	if request.Answers.Docs != "" && request.Answers.NoDocs {
		return Result{}, errors.New("setup: --docs and --no-docs cannot be combined")
	}
	if request.Answers.Decisions != "" && request.Answers.NoDecisions {
		return Result{}, errors.New("setup: --decisions and --no-decisions cannot be combined")
	}
	if request.Interactive && request.Prompter != nil && request.Catalog == nil {
		return Result{}, errors.New("setup: a prompt catalog is required")
	}

	resolved := state.State{SchemaVersion: state.SchemaVersion}
	if request.Existing != nil {
		resolved = *request.Existing
	}
	pending := pendingSet(request.Existing)

	cliLanguage := request.CLILanguage
	if cliLanguage == "" {
		cliLanguage = string(i18n.English)
	}

	strategy, err := resolveHost(request, &resolved, pending)
	if err != nil {
		return Result{}, err
	}
	if err := resolveLanguage(request, &resolved, pending, cliLanguage); err != nil {
		return Result{}, err
	}
	resolved.Profile = resolveProfile(request)
	if err := resolveDocs(request, &resolved, pending); err != nil {
		return Result{}, err
	}
	if err := resolveDecisions(request, &resolved, pending); err != nil {
		return Result{}, err
	}
	bundles, preset, err := resolveBundles(request, &resolved, pending)
	if err != nil {
		return Result{}, err
	}
	resolved.SpecEngine = resolveSpecEngine(request.Project, request.Existing)

	ordered := sortPending(pending)
	resolved.Setup = state.Setup{Complete: len(ordered) == 0, Pending: ordered}
	return Result{
		State:    resolved,
		Pending:  ordered,
		Complete: len(ordered) == 0,
		Bundles:  bundles,
		Preset:   preset,
		Strategy: strategy,
	}, nil
}

func resolveHost(request Request, resolved *state.State, pending map[string]bool) (host.Strategy, error) {
	if request.Answers.Host != "" {
		strategy, err := host.Lookup(request.Answers.Host)
		if err != nil {
			return nil, err
		}
		resolved.Host = state.Host{ID: strategy.ID(), Source: "flag", Evidence: []string{}}
		delete(pending, PendingHost)
		return strategy, nil
	}

	if request.Existing != nil && request.Existing.Host.ID != "" && !pending[PendingHost] {
		strategy, err := host.Lookup(request.Existing.Host.ID)
		if err != nil {
			return nil, err
		}
		resolved.Host = request.Existing.Host
		return strategy, nil
	}

	resolution, err := host.Resolve(request.Project, "", request.Getenv)
	if err != nil {
		return nil, err
	}
	if !resolution.NeedsQuestion() && resolution.Host != "" {
		strategy, err := host.Lookup(resolution.Host)
		if err != nil {
			return nil, err
		}
		resolved.Host = state.Host{
			ID:       resolution.Host,
			Source:   hostSource(resolution.Reason),
			Evidence: hostEvidence(resolution.Evidence),
		}
		delete(pending, PendingHost)
		return strategy, nil
	}

	if request.Interactive && request.Prompter != nil {
		answer, err := ask(request, "setup.prompt.host", map[string]string{
			"choices": joinChoices(host.IDs()),
			"default": "none",
		}, "none")
		if err != nil {
			return nil, err
		}
		if answer != "" && answer != "none" {
			strategy, err := host.Lookup(answer)
			if err != nil {
				return nil, err
			}
			resolved.Host = state.Host{ID: strategy.ID(), Source: "question", Evidence: []string{}}
			delete(pending, PendingHost)
			return strategy, nil
		}
	}

	pending[PendingHost] = true
	resolved.Host = state.Host{Evidence: []string{}}
	return nil, nil
}

func resolveLanguage(request Request, resolved *state.State, pending map[string]bool, cliLanguage string) error {
	if request.LangSet {
		resolved.Language = state.Language{CLI: cliLanguage, User: cliLanguage, Agent: "en"}
		delete(pending, PendingLanguage)
		return nil
	}

	if request.Existing != nil && request.Existing.Language.User != "" && !pending[PendingLanguage] {
		resolved.Language = request.Existing.Language
		return nil
	}

	if request.Interactive && request.Prompter != nil {
		answer, err := ask(request, "setup.prompt.language", map[string]string{"default": cliLanguage}, cliLanguage)
		if err != nil {
			return err
		}
		language, ok := i18n.Parse(answer)
		if !ok {
			return fmt.Errorf("setup: unsupported language %q", answer)
		}
		resolved.Language = state.Language{CLI: cliLanguage, User: string(language), Agent: "en"}
		delete(pending, PendingLanguage)
		return nil
	}

	resolved.Language = state.Language{CLI: cliLanguage, User: cliLanguage, Agent: "en"}
	pending[PendingLanguage] = true
	return nil
}

func resolveProfile(request Request) string {
	if request.Answers.Profile != "" {
		return request.Answers.Profile
	}
	if request.Existing != nil && request.Existing.Profile != "" {
		return request.Existing.Profile
	}
	return detectProfile(request.Project)
}

func resolveSpecEngine(project string, existing *state.State) state.SpecEngine {
	if existing != nil && existing.SpecEngine.ID != "" {
		return existing.SpecEngine
	}
	return spec.Detect(project)
}

func hostSource(reason host.Reason) string {
	switch reason {
	case host.ReasonFlag:
		return "flag"
	case host.ReasonProject:
		return "evidence"
	case host.ReasonEnvironment, host.ReasonEnvironmentAndProject:
		return "env"
	default:
		return ""
	}
}

func hostEvidence(evidence []host.Evidence) []string {
	set := map[string]bool{}
	for _, item := range evidence {
		for _, signal := range item.Signals {
			set[signal] = true
		}
		for _, path := range item.Paths {
			set[path] = true
		}
	}
	return sortedStrings(set)
}
