package server

import (
	"teleport/internal/app"
	"teleport/internal/model"
)

func init() {
	entries := []struct {
		name, action, summary string
		input, output         any
	}{
		{"configureLocalRepository", "ACT-1017", "Administrator grants an exact local repository and storage root", app.RepositoryInput{}, model.Project{}},
		{"prepareTaskWorktree", "ACT-0414", "Prepare or reconcile an owned retained worktree", app.GitTaskInput{}, model.Ticket{}},
		{"recordTaskDelivery", "ACT-0418", "Capture a real external commit and controller checks", app.GitTaskInput{}, model.Ticket{}},
		{"reconcileGitOperation", "ACT-0319", "Read retained Git facts after verified controller exit", app.VersionInput{}, model.Ticket{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
