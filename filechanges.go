package tail

type fileChanges struct {
	modified  chan bool // Channel to get notified of modifications
	Truncated chan bool // Channel to get notified of truncations
	deleted   chan bool // Channel to get notified of deletions/renames
}

func newFileChanges() *fileChanges {
	return &fileChanges{
		make(chan bool, 1), make(chan bool, 1), make(chan bool, 1)}
}

func (fc *fileChanges) notifyModified() {
	sendOnlyIfEmpty(fc.modified)
}

func (fc *fileChanges) notifyTruncated() {
	sendOnlyIfEmpty(fc.Truncated)
}

func (fc *fileChanges) notifyDeleted() {
	sendOnlyIfEmpty(fc.deleted)
}

// sendOnlyIfEmpty sends on a bool channel only if the channel has no
// backlog to be read by other goroutines. This concurrency pattern
// can be used to notify other goroutines if and only if they are
// looking for it (i.e., subsequent notifications can be compressed
// into one).
func sendOnlyIfEmpty(ch chan bool) {
	select {
	case ch <- true:
	default:
	}
}
