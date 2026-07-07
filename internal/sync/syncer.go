package sync

// GitSyncer manages git operations for data synchronisation.
type GitSyncer interface {
	Pull() error
	Add(paths []string) error
	Commit(message string) error
	Push(force bool) error
}
