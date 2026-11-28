package spec

// noneAdapter is a project without a specification engine: no directory, no
// operations. Its operations report ErrNotApplicable so the command layer can
// inform the human without treating it as a failure.
type noneAdapter struct{}

func (noneAdapter) ID() string { return "none" }

func (noneAdapter) Root() string { return "" }

func (noneAdapter) Validate(string) error { return ErrNotApplicable }

func (noneAdapter) Archive(string) error { return ErrNotApplicable }

func (noneAdapter) Sync(string) error { return ErrNotApplicable }
