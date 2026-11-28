package setup

import (
	"strings"

	"github.com/cedaniel200/atlasctl/internal/state"
)

var docsCandidates = []string{"docs", "documentation"}
var decisionsCandidates = []string{"adr", "decisions"}

func resolveDocs(request Request, resolved *state.State, pending map[string]bool) error {
	switch {
	case request.Answers.NoDocs:
		resolved.Layout.Docs = ""
		delete(pending, PendingDocs)
		return nil
	case request.Answers.Docs != "":
		resolved.Layout.Docs = request.Answers.Docs
		delete(pending, PendingDocs)
		return nil
	case request.Existing != nil && !pending[PendingDocs]:
		resolved.Layout.Docs = request.Existing.Layout.Docs
		return nil
	}

	candidates := existingDirs(request.Project, docsCandidates)
	if len(candidates) == 1 {
		resolved.Layout.Docs = candidates[0]
		delete(pending, PendingDocs)
		return nil
	}

	if request.Interactive && request.Prompter != nil {
		answer, err := ask(request, "setup.prompt.docs", map[string]string{
			"choices": joinChoices(candidates),
			"default": "none",
		}, "none")
		if err != nil {
			return err
		}
		if answer == "" || answer == "none" {
			resolved.Layout.Docs = ""
		} else {
			resolved.Layout.Docs = answer
		}
		delete(pending, PendingDocs)
		return nil
	}

	resolved.Layout.Docs = ""
	pending[PendingDocs] = true
	return nil
}

func resolveDecisions(request Request, resolved *state.State, pending map[string]bool) error {
	switch {
	case request.Answers.NoDecisions:
		resolved.Layout.Decisions = nil
		delete(pending, PendingDecisions)
		return nil
	case request.Answers.Decisions != "":
		value := request.Answers.Decisions
		resolved.Layout.Decisions = &value
		delete(pending, PendingDecisions)
		return nil
	case request.Existing != nil && !pending[PendingDecisions]:
		resolved.Layout.Decisions = request.Existing.Layout.Decisions
		return nil
	}

	candidates := existingDirs(request.Project, decisionsCandidates)
	if len(candidates) == 1 {
		value := candidates[0]
		resolved.Layout.Decisions = &value
		delete(pending, PendingDecisions)
		return nil
	}

	if request.Interactive && request.Prompter != nil {
		answer, err := ask(request, "setup.prompt.decisions", map[string]string{
			"choices": joinChoices(candidates),
			"default": "none",
		}, "none")
		if err != nil {
			return err
		}
		if answer == "" || answer == "none" {
			resolved.Layout.Decisions = nil
		} else {
			value := answer
			resolved.Layout.Decisions = &value
		}
		delete(pending, PendingDecisions)
		return nil
	}

	resolved.Layout.Decisions = nil
	pending[PendingDecisions] = true
	return nil
}

func joinChoices(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
