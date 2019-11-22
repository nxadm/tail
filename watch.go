// Copyright (c) 2015 HPE Software Inc. All rights reserved.
// Copyright (c) 2013 ActiveState Software Inc. All rights reserved.

package tail

import (
	"context"
)

// fileWatcher monitors file-level events.
type fileWatcher interface {
	// BlockUntilExistsOLD blocks until the file comes into existence.
	blockUntilExists(ctx context.Context) error

	// ChangeEventsOLD reports on changes to a file, be it modification,
	// deletion, renames or truncations. Returned fileChanges group of
	// channels will be closed, thus become unusable, after a deletion
	// or truncation event.
	// In order to properly report truncations, ChangeEventsOLD requires
	// the caller to pass their current offset in the file.
	changeEvents(ctx context.Context, pos int64) (*fileChanges, error)
}
