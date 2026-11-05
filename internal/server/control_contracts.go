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
		{"setDependencies", "ACT-0415", "Set versioned prerequisites", app.DependenciesInput{}, model.Ticket{}},
		{"cancelTicket", "ACT-0311", "Cancel selected work or unfinished subtree", app.VersionInput{}, model.Ticket{}},
		{"reopenTicket", "ACT-0310", "Explicitly reopen selected terminal work", app.VersionInput{}, model.Ticket{}},
		{"setTicketVisibility", "ACT-0106", "Change selected visibility with no cascade", app.VisibilityInput{}, model.Ticket{}},
		{"transferTicket", "ACT-0208", "Transfer a ticket or subtree between owned projects", app.TransferInput{}, model.Ticket{}},
		{"previewTicketTransfer", "ACT-0208", "Preview a ticket's project transfer and affected hierarchy", app.TransferPreviewInput{}, app.TransferPreview{}},
		{"moveTicket", "ACT-0201", "Reparent within the same project", app.MoveInput{}, model.Ticket{}},
		{"changePassword", "ACT-1317", "Change password and revoke sign-in sessions", app.PasswordInput{}, struct {
			SignedOut bool `json:"signedOut"`
		}{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
