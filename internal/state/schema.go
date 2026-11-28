package state

import (
	"errors"
	"fmt"
)

// ErrUnsupportedSchema reports a state or manifest written by a newer engine
// than this binary knows how to handle.
var ErrUnsupportedSchema = errors.New("unsupported schema version")

func checkReadableSchema(version int) error {
	if version > SchemaVersion {
		return fmt.Errorf(
			"%w: found %d, supported %d; install a compatible atlasctl binary",
			ErrUnsupportedSchema, version, SchemaVersion,
		)
	}
	return nil
}

func requireWritableSchema(version int) error {
	if err := checkReadableSchema(version); err != nil {
		return err
	}
	if version != SchemaVersion {
		return fmt.Errorf("refusing to write schema version %d; this binary writes version %d", version, SchemaVersion)
	}
	return nil
}
