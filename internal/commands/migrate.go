package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cedaniel200/atlasctl/internal/bundle"
	"github.com/cedaniel200/atlasctl/internal/content"
	"github.com/cedaniel200/atlasctl/internal/fsutil"
	"github.com/cedaniel200/atlasctl/internal/hooks"
	"github.com/cedaniel200/atlasctl/internal/setup"
	"github.com/cedaniel200/atlasctl/internal/spec"
	"github.com/cedaniel200/atlasctl/internal/state"
)

// ErrConfirmationRequired reports a migration that needs an explicit --yes in
// a non-interactive invocation.
var ErrConfirmationRequired = errors.New("commands: migration requires confirmation; pass --yes in non-interactive mode")

// ErrMigrationCancelled reports a migration cancelled by the human.
var ErrMigrationCancelled = errors.New("commands: migration cancelled")

// MigratePlan is what the human confirms before any write.
type MigratePlan struct {
	Files     int
	Scripts   int
	RemoveNew []string
	KeepNew   []string
}

// MigrateRequest is the input of a v1 to v2 migration.
type MigrateRequest struct {
	Project  string
	Snapshot string
	Confirm  func(MigratePlan) (bool, error)
}

// MigrateResult is the outcome of a migration.
type MigrateResult struct {
	Already   bool
	BackupDir string
	Plan      MigratePlan
	State     *state.State
	Manifest  *state.Manifest
}

// Migrate converts the state and manifest of an adopted project from schema v1
// to v2. It validates everything and takes a backup before the first write. An
// already migrated project is reported without a backup.
func Migrate(request MigrateRequest) (MigrateResult, error) {
	if request.Project == "" {
		return MigrateResult{}, errors.New("commands: project root is required")
	}
	statePath := filepath.Join(request.Project, ".atlas", "state.json")
	manifestPath := filepath.Join(request.Project, ".atlas", "manifest.json")
	for _, relative := range []string{".atlas/state.json", ".atlas/manifest.json"} {
		if err := fsutil.CheckNoSymlinks(request.Project, relative); err != nil {
			return MigrateResult{}, migrationError(err)
		}
	}
	stateBytes, err := os.ReadFile(statePath)
	if errors.Is(err, fs.ErrNotExist) {
		return MigrateResult{}, migrationError(errors.New("state is missing"))
	}
	if err != nil {
		return MigrateResult{}, migrationError(err)
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return MigrateResult{}, migrationError(errors.New("manifest is missing"))
	}
	if err != nil {
		return MigrateResult{}, migrationError(err)
	}

	loadedState, err := state.Load(statePath)
	if err != nil {
		return MigrateResult{}, migrationError(err)
	}
	loadedManifest, err := state.LoadManifest(manifestPath)
	if err != nil {
		return MigrateResult{}, migrationError(err)
	}
	if loadedState.SchemaVersion == state.SchemaVersion && loadedManifest.SchemaVersion == state.SchemaVersion {
		return MigrateResult{Already: true, State: loadedState, Manifest: loadedManifest}, nil
	}
	if loadedState.SchemaVersion != 1 || loadedManifest.SchemaVersion != 1 {
		return MigrateResult{}, migrationError(fmt.Errorf(
			"state schema %d and manifest schema %d disagree",
			loadedState.SchemaVersion, loadedManifest.SchemaVersion,
		))
	}
	if err := validateLegacyState(loadedState); err != nil {
		return MigrateResult{}, migrationError(err)
	}
	if err := validateLegacyManifest(loadedManifest); err != nil {
		return MigrateResult{}, migrationError(err)
	}

	files := len(loadedManifest.Files)
	translatedState, translatedManifest, dispatcher, scripts := translateMigration(request.Project, loadedState, loadedManifest)
	removeNew, keepNew, err := planNewFiles(request.Project, request.Snapshot)
	if err != nil {
		return MigrateResult{}, migrationError(err)
	}
	plan := MigratePlan{
		Files:     files,
		Scripts:   scripts,
		RemoveNew: removeNew,
		KeepNew:   keepNew,
	}
	dispatcherState, err := inspectDispatcher(request.Project, dispatcher)
	if err != nil {
		return MigrateResult{}, migrationError(err)
	}
	if dispatcherState.divergent {
		return MigrateResult{}, migrationError(fmt.Errorf(
			"%s differs from the expected dispatcher; move it aside and retry",
			hooks.DispatcherPath,
		))
	}
	if request.Confirm != nil {
		confirmed, err := request.Confirm(plan)
		if err != nil {
			return MigrateResult{}, err
		}
		if !confirmed {
			return MigrateResult{}, ErrMigrationCancelled
		}
	}

	backupDir, err := createBackup(request.Project, stateBytes, manifestBytes)
	if err != nil {
		return MigrateResult{}, migrationError(err)
	}
	if err := writeDispatcher(request.Project, dispatcher, &dispatcherState); err != nil {
		return MigrateResult{}, migrationError(err)
	}
	hookStatus, err := hooks.Status(request.Project)
	if err != nil {
		rollbackDispatcher(request.Project, dispatcherState)
		return MigrateResult{}, migrationError(err)
	}
	translatedState.Hooks.Installed = hookStatus.Active
	if err := state.SaveManifest(manifestPath, translatedManifest); err != nil {
		rollbackDispatcher(request.Project, dispatcherState)
		return MigrateResult{}, migrationError(err)
	}
	if err := state.Save(statePath, translatedState); err != nil {
		rollbackManifest(manifestPath, manifestBytes)
		rollbackDispatcher(request.Project, dispatcherState)
		return MigrateResult{}, migrationError(err)
	}

	result := MigrateResult{
		BackupDir: backupDir,
		Plan:      plan,
		State:     translatedState,
		Manifest:  translatedManifest,
	}
	removed := make([]string, 0, len(plan.RemoveNew))
	for _, relative := range plan.RemoveNew {
		if err := fsutil.RemoveProjectFile(request.Project, relative); err != nil {
			return result, fmt.Errorf("commands: migrated, but could not remove %q: %w", relative, err)
		}
		removed = append(removed, relative)
	}
	result.Plan.RemoveNew = removed
	return result, nil
}

func migrationError(err error) error {
	return fmt.Errorf("commands: cannot migrate: %w; back up the project and reinstall with create or adopt", err)
}

func validateLegacyState(value *state.State) error {
	if value.SchemaVersion != 1 {
		return fmt.Errorf("state schema %d is not v1", value.SchemaVersion)
	}
	return nil
}

func validateLegacyManifest(manifest *state.Manifest) error {
	if len(manifest.Files) == 0 {
		return errors.New("manifest has no files")
	}
	for path, entry := range manifest.Files {
		if err := fsutil.ValidateRelative(path); err != nil {
			return fmt.Errorf("manifest path %q: %w", path, err)
		}
		if entry.Origin == "" {
			return fmt.Errorf("manifest %q has no origin", path)
		}
		if !validSHA256(entry.SHA256) {
			return fmt.Errorf("manifest %q has an invalid sha256", path)
		}
		switch entry.Layer {
		case state.LayerBase, state.LayerOwned, state.LayerGenerated:
		default:
			return fmt.Errorf("manifest %q has an unknown layer %q", path, entry.Layer)
		}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func translateMigration(project string, loadedState *state.State, loadedManifest *state.Manifest) (*state.State, *state.Manifest, []byte, int) {
	translatedState := *loadedState
	translatedState.SchemaVersion = state.SchemaVersion
	translatedState.EngineVersion = EngineVersion
	translatedState.Bundles = []string{setup.DefaultBundle}
	translatedState.SpecEngine = spec.Detect(project)
	translatedState.MaxParallel = state.MaxParallelAuto
	if translatedState.Content.BundleVersion == "" {
		translatedState.Content.BundleVersion = translatedState.EngineVersion
	}
	translatedState.Setup = migrationSetup(translatedState)

	translatedManifest := *loadedManifest
	translatedManifest.SchemaVersion = state.SchemaVersion
	translatedManifest.EngineVersion = EngineVersion
	translatedManifest.GeneratedAt = time.Now().UTC()
	translatedManifest.Bundles = []string{setup.DefaultBundle}
	scripts := 0
	for path := range translatedManifest.Files {
		if !strings.HasPrefix(path, ".atlas/scripts/") {
			continue
		}
		entry := translatedManifest.Files[path]
		entry.Layer = state.LayerOwned
		translatedManifest.Files[path] = entry
		scripts++
	}
	dispatcher := hooks.Script(translatedState.SpecEngine.Root)
	translatedManifest.Files[hooks.DispatcherPath] = state.ManifestFile{
		Bundle: setup.DefaultBundle,
		Origin: "core",
		Layer:  state.LayerGenerated,
		SHA256: content.Checksum(dispatcher),
	}
	return &translatedState, &translatedManifest, dispatcher, scripts
}

// planNewFiles finds every .new file in the project and compares it with the
// incoming snapshot. Only a byte-equal (LF-normalized) copy is scheduled for
// deletion; anything that cannot be compared is kept.
func planNewFiles(project, snapshotPath string) ([]string, []string, error) {
	candidates, err := findNewFiles(project)
	if err != nil {
		return nil, nil, err
	}
	if len(candidates) == 0 {
		return nil, nil, nil
	}
	snapshot, err := bundle.Load(snapshotPath)
	if err != nil {
		return nil, nil, err
	}

	var remove, keep []string
	for _, relative := range candidates {
		basePath := strings.TrimSuffix(relative, ".new")
		file, ok := snapshot.Read(setup.DefaultBundle, basePath)
		if !ok {
			keep = append(keep, relative)
			continue
		}
		absolute, err := fsutil.SafeJoin(project, relative)
		if err != nil {
			return nil, nil, err
		}
		info, err := os.Lstat(absolute)
		if err != nil || !info.Mode().IsRegular() {
			keep = append(keep, relative)
			continue
		}
		data, err := os.ReadFile(absolute)
		if err != nil {
			keep = append(keep, relative)
			continue
		}
		if content.Checksum(file.Data) == content.Checksum(data) {
			remove = append(remove, relative)
		} else {
			keep = append(keep, relative)
		}
	}
	sort.Strings(remove)
	sort.Strings(keep)
	return remove, keep, nil
}

func findNewFiles(project string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(project, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(project, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if entry.Name() == ".git" || relative == ".atlas/backups" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(relative, ".new") {
			found = append(found, relative)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

func migrationSetup(value state.State) state.Setup {
	if value.Setup.Complete {
		return state.Setup{Complete: true, Pending: []string{}}
	}
	var pending []string
	if value.Host.ID == "" {
		pending = append(pending, setup.PendingHost)
	}
	if value.Language.User == "" {
		pending = append(pending, setup.PendingLanguage)
	}
	if value.Layout.Docs == "" {
		pending = append(pending, setup.PendingDocs)
	}
	if len(value.Bundles) == 0 {
		pending = append(pending, setup.PendingBundles)
	}
	return state.Setup{Complete: len(pending) == 0, Pending: pending}
}

type migrationDispatcher struct {
	existed   bool
	mode      fs.FileMode
	created   bool
	chmodded  bool
	divergent bool
}

func inspectDispatcher(project string, data []byte) (migrationDispatcher, error) {
	absolute, err := fsutil.SafeJoin(project, hooks.DispatcherPath)
	if err != nil {
		return migrationDispatcher{}, err
	}
	if err := fsutil.CheckNoSymlinks(project, hooks.DispatcherPath); err != nil {
		return migrationDispatcher{}, err
	}
	info, err := os.Stat(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return migrationDispatcher{}, nil
	}
	if err != nil {
		return migrationDispatcher{}, err
	}
	if !info.Mode().IsRegular() {
		return migrationDispatcher{}, fmt.Errorf("%s is not a regular file", hooks.DispatcherPath)
	}
	existing, err := os.ReadFile(absolute)
	if err != nil {
		return migrationDispatcher{}, err
	}
	status := migrationDispatcher{existed: true, mode: info.Mode().Perm()}
	if content.Checksum(existing) != content.Checksum(data) {
		status.divergent = true
	}
	return status, nil
}

func writeDispatcher(project string, data []byte, status *migrationDispatcher) error {
	switch {
	case !status.existed:
		if err := fsutil.WriteProjectFileMode(project, hooks.DispatcherPath, data, 0o755); err != nil {
			return err
		}
		status.created = true
	case !status.divergent && status.mode.Perm()&0o111 == 0:
		absolute, err := fsutil.SafeJoin(project, hooks.DispatcherPath)
		if err != nil {
			return err
		}
		if err := os.Chmod(absolute, 0o755); err != nil {
			return err
		}
		status.chmodded = true
	}
	return nil
}

func rollbackDispatcher(project string, status migrationDispatcher) {
	switch {
	case status.created:
		_ = fsutil.RemoveProjectFile(project, hooks.DispatcherPath)
	case status.chmodded:
		absolute, err := fsutil.SafeJoin(project, hooks.DispatcherPath)
		if err == nil {
			_ = os.Chmod(absolute, status.mode)
		}
	}
}

func rollbackManifest(path string, data []byte) {
	_ = os.WriteFile(path, data, 0o644)
}

func createBackup(project string, stateBytes, manifestBytes []byte) (string, error) {
	if err := fsutil.CheckNoSymlinks(project, ".atlas/backups"); err != nil {
		return "", err
	}
	root := filepath.Join(project, ".atlas", "backups")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	directory := filepath.Join(root, stamp)
	for suffix := 1; ; suffix++ {
		err := os.Mkdir(directory, 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		directory = filepath.Join(root, fmt.Sprintf("%s-%d", stamp, suffix))
	}
	if err := os.WriteFile(filepath.Join(directory, "state.json"), stateBytes, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), manifestBytes, 0o644); err != nil {
		return "", err
	}
	return directory, nil
}
