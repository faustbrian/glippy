package lsp

import (
	"fmt"
	"testing"
)

func TestQueueWorkspaceChangesPromotesOverflowToFullInvalidation(t *testing.T) {
	t.Parallel()

	server := &server{}
	paths := make([]string, maximumWatchedFileChanges)
	for index := range paths {
		paths[index] = fmt.Sprintf("/project/%04d.go", index)
	}
	server.queueWorkspaceChanges(WorkspaceFileChanges{Paths: paths})
	server.queueWorkspaceChanges(WorkspaceFileChanges{Paths: []string{"/project/overflow.go"}})

	if !server.pendingWorkspaceChanges.InvalidateAll ||
		len(server.pendingWorkspaceChanges.Paths) != 0 {
		t.Fatalf(
			"queued overflow = %#v, want explicit full invalidation",
			server.pendingWorkspaceChanges,
		)
	}
}

func TestWatchedFilePathsPromotesSingleOverflowToFullInvalidation(t *testing.T) {
	t.Parallel()

	changes := make([]watchedFileChange, maximumWatchedFileChanges + 1)
	for index := range changes {
		changes[index] = watchedFileChange{
			URI: fmt.Sprintf("file:///project/%04d.go", index),
			Type: 2,
		}
	}
	result, err := watchedFileChanges(changes)
	if err != nil {
		t.Fatalf("watchedFileChanges(single overflow) error = %v", err)
	}
	if !result.InvalidateAll || len(result.Paths) != 0 {
		t.Fatalf(
			"watchedFileChanges(single overflow) = %#v, want full invalidation",
			result,
		)
	}
}
