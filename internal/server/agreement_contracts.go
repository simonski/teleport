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
		{"getTicketAgreements", "ACT-0304", "Read current and saved acceptance bases", app.IDInput{}, app.AgreementBasis{}},
		{"markReady", "ACT-0303", "Reconfirm exact current readiness after a basis change", app.MarkReadyInput{}, model.Ticket{}},
		{"rejectRefinement", "ACT-0301", "Reject a proposal along its declared failure route", app.VersionInput{}, model.Ticket{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
