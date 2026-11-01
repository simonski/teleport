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
		{"createProject", "ACT-0504", "Create a project with protected defaults", app.CreateProjectInput{}, model.Project{}},
		{"listProjects", "ACT-0502", "List accessible projects", app.ListInput{}, app.Page[model.Project]{}},
		{"createTicket", "ACT-0101", "Create a Todo ticket", app.CreateTicketInput{}, model.Ticket{}},
		{"getTicket", "ACT-0102", "Read a ticket", app.IDInput{}, model.Ticket{}},
		{"listTickets", "ACT-0103", "List authorized tickets", app.ListInput{}, app.Page[model.Ticket]{}},
		{"updateTicket", "ACT-0104", "Edit permitted ticket fields", app.UpdateTicketInput{}, model.Ticket{}},
		{"transitionTicket", "ACT-0301", "Move a human ticket with an exact agreement", app.TransitionInput{}, model.Ticket{}},
		{"listTicketHistory", "ACT-0801", "Read immutable ticket revisions", app.IDInput{}, app.Page[model.Revision]{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
