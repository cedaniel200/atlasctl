package host

// Union is the set of project routes, configuration files and legacy
// directories an application needs when the active host changes.
type Union struct {
	Allowed     []string
	ConfigFiles []string
	LegacyDirs  []string
}

// Transition returns the union of the previous and next hosts. Applying a host
// change needs both: the previous host's generated files are deleted while the
// next host's are written, and both configuration files must stay excluded from
// pruning. A nil strategy is ignored, so a first adoption passes only the new
// host.
func Transition(previous, next Strategy) Union {
	strategies := make([]Strategy, 0, 2)
	if previous != nil {
		strategies = append(strategies, previous)
	}
	if next != nil {
		strategies = append(strategies, next)
	}

	var union Union
	for _, strategy := range strategies {
		union.Allowed = append(union.Allowed, strategy.AllowedRoutes()...)
		union.ConfigFiles = append(union.ConfigFiles, strategy.ConfigFiles()...)
		union.LegacyDirs = append(union.LegacyDirs, strategy.LegacyDirs()...)
	}
	union.Allowed = uniqueSorted(union.Allowed)
	union.ConfigFiles = uniqueSorted(union.ConfigFiles)
	union.LegacyDirs = uniqueSorted(union.LegacyDirs)
	return union
}
