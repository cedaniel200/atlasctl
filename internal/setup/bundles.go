package setup

import (
	"strings"

	"github.com/cedaniel200/atlasctl/internal/bundle"
	"github.com/cedaniel200/atlasctl/internal/state"
)

func resolveBundles(request Request, resolved *state.State, pending map[string]bool) ([]string, string, error) {
	if len(request.Answers.Bundles) > 0 || request.Answers.Preset != "" {
		bundles := uniqueStrings(request.Answers.Bundles)
		resolved.Bundles = bundles
		delete(pending, PendingBundles)
		return bundles, request.Answers.Preset, nil
	}

	if request.Existing != nil && len(request.Existing.Bundles) > 0 && !pending[PendingBundles] {
		resolved.Bundles = append([]string(nil), request.Existing.Bundles...)
		return resolved.Bundles, "", nil
	}

	if request.Interactive && request.Prompter != nil {
		answer, err := ask(request, "setup.prompt.bundles", map[string]string{"default": DefaultBundle}, DefaultBundle)
		if err != nil {
			return nil, "", err
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			answer = DefaultBundle
		}
		if _, ok := bundle.Presets()[answer]; ok {
			resolved.Bundles = nil
			delete(pending, PendingBundles)
			return nil, answer, nil
		}
		bundles := splitBundles(answer)
		resolved.Bundles = bundles
		delete(pending, PendingBundles)
		return bundles, "", nil
	}

	resolved.Bundles = []string{DefaultBundle}
	pending[PendingBundles] = true
	return resolved.Bundles, "", nil
}

func splitBundles(value string) []string {
	bundles := make([]string, 0, 2)
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			bundles = append(bundles, item)
		}
	}
	if len(bundles) == 0 {
		return []string{DefaultBundle}
	}
	return uniqueStrings(bundles)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	if len(unique) == 0 {
		return []string{DefaultBundle}
	}
	return unique
}
