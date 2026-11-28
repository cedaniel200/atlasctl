package host

import (
	"os"
	"path/filepath"
	"sort"
)

// Verification reports whether a host's detection was checked against a real
// installation. Only opencode is verified; claude, cursor and codex follow
// their documented contracts and remain unverified until someone installs them.
type Verification string

const (
	// Verified means the host signals were checked in this environment.
	Verified Verification = "verified"
	// Unverified means the host signals follow its documented contract but
	// have not been checked against a real installation.
	Unverified Verification = "unverified"
)

// Reason explains why Resolve chose a host or why it must ask the human.
type Reason string

const (
	// ReasonFlag means the host came from an explicit --host value.
	ReasonFlag Reason = "flag"
	// ReasonEnvironment means only environment signals were found.
	ReasonEnvironment Reason = "environment"
	// ReasonProject means only project evidence was found.
	ReasonProject Reason = "project"
	// ReasonEnvironmentAndProject means both agreed on the same host.
	ReasonEnvironmentAndProject Reason = "environment_and_project"
	// ReasonContradiction means signals and evidence disagree or point to more
	// than one host. It is never resolved by priority.
	ReasonContradiction Reason = "contradiction"
	// ReasonNoEvidence means neither signals nor project evidence were found.
	ReasonNoEvidence Reason = "no_evidence"
)

// Evidence is what a host detection found. Signals are environment variable
// names; Paths are project-relative directories.
type Evidence struct {
	Host    string
	Signals []string
	Paths   []string
}

// Resolution is the outcome of host detection.
type Resolution struct {
	Host     string
	Reason   Reason
	Evidence []Evidence
}

// NeedsQuestion reports whether the caller must ask the human instead of
// choosing a host.
func (r Resolution) NeedsQuestion() bool {
	return r.Reason == ReasonContradiction || r.Reason == ReasonNoEvidence
}

// Detect implements Strategy.
func (opencode) Detect(project string, getenv func(string) string) (Evidence, bool) {
	return detectHost(project, getenv, openCodeID,
		[]string{"OPENCODE", "OPENCODE_SESSION_ID", "OPENCODE_TERMINAL"}, ".opencode")
}

// Verification implements Strategy.
func (opencode) Verification() Verification { return Verified }

// Detect implements Strategy.
func (claude) Detect(project string, getenv func(string) string) (Evidence, bool) {
	return detectHost(project, getenv, claudeID, []string{"CLAUDECODE"}, ".claude")
}

// Verification implements Strategy.
func (claude) Verification() Verification { return Unverified }

// Detect implements Strategy. Cursor is also signalled by TERM_PROGRAM=Cursor,
// which is not an environment variable whose presence is enough: its value must
// be Cursor.
func (cursor) Detect(project string, getenv func(string) string) (Evidence, bool) {
	evidence, found := detectHost(project, getenv, cursorID, []string{"CURSOR_TRACE_ID"}, ".cursor")
	if readEnv(getenv, "TERM_PROGRAM") == "Cursor" {
		evidence.Signals = append(evidence.Signals, "TERM_PROGRAM=Cursor")
		found = true
	}
	return evidence, found
}

// Verification implements Strategy.
func (cursor) Verification() Verification { return Unverified }

// Detect implements Strategy. Codex has no known environment signal; only the
// .codex/ directory is evidence.
func (codex) Detect(project string, getenv func(string) string) (Evidence, bool) {
	return detectHost(project, getenv, codexID, nil, ".codex")
}

// Verification implements Strategy.
func (codex) Verification() Verification { return Unverified }

func detectHost(project string, getenv func(string) string, id string, signals []string, evidenceDir string) (Evidence, bool) {
	evidence := Evidence{Host: id}
	for _, name := range signals {
		if readEnv(getenv, name) != "" {
			evidence.Signals = append(evidence.Signals, name)
		}
	}
	if project != "" && evidenceDir != "" {
		path := filepath.Join(project, evidenceDir)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			evidence.Paths = append(evidence.Paths, evidenceDir)
		}
	}
	return evidence, len(evidence.Signals) > 0 || len(evidence.Paths) > 0
}

func readEnv(getenv func(string) string, name string) string {
	if getenv == nil {
		return ""
	}
	return getenv(name)
}

// Resolve chooses a host with the order environment -> project -> question. A
// non-empty flagHost resolves immediately without evaluating any signal. When
// signals and project evidence disagree, or when more than one host matches,
// Resolve asks the human instead of breaking the tie by priority.
func Resolve(project, flagHost string, getenv func(string) string) (Resolution, error) {
	if flagHost != "" {
		strategy, err := Lookup(flagHost)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{Host: strategy.ID(), Reason: ReasonFlag}, nil
	}

	found := make([]Evidence, 0, len(registry))
	for _, strategy := range registry {
		evidence, ok := strategy.Detect(project, getenv)
		if !ok {
			continue
		}
		evidence.Host = strategy.ID()
		found = append(found, evidence)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Host < found[j].Host })

	signalHosts := hostsWith(found, func(evidence Evidence) bool { return len(evidence.Signals) > 0 })
	pathHosts := hostsWith(found, func(evidence Evidence) bool { return len(evidence.Paths) > 0 })

	switch {
	case len(signalHosts) == 1 && len(pathHosts) == 0:
		return Resolution{Host: signalHosts[0], Reason: ReasonEnvironment, Evidence: found}, nil
	case len(signalHosts) == 0 && len(pathHosts) == 1:
		return Resolution{Host: pathHosts[0], Reason: ReasonProject, Evidence: found}, nil
	case len(signalHosts) == 1 && len(pathHosts) == 1 && signalHosts[0] == pathHosts[0]:
		return Resolution{Host: signalHosts[0], Reason: ReasonEnvironmentAndProject, Evidence: found}, nil
	case len(found) == 0:
		return Resolution{Reason: ReasonNoEvidence}, nil
	default:
		return Resolution{Reason: ReasonContradiction, Evidence: found}, nil
	}
}

func hostsWith(evidence []Evidence, match func(Evidence) bool) []string {
	seen := make(map[string]struct{}, len(evidence))
	hosts := make([]string, 0, len(evidence))
	for _, item := range evidence {
		if !match(item) {
			continue
		}
		if _, ok := seen[item.Host]; ok {
			continue
		}
		seen[item.Host] = struct{}{}
		hosts = append(hosts, item.Host)
	}
	return hosts
}
