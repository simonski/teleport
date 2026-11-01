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
		{"saveIdea", "ACT-0614", "Create or edit a human Idea", app.SaveIdeaInput{}, model.Idea{}},
		{"listIdeas", "ACT-0613", "List currently authorized Ideas", app.ListIdeasInput{}, app.Page[model.Idea]{}},
		{"getIdea", "ACT-0613", "Read an Idea and authorized proposal provenance", app.IdeaInput{}, app.IdeaDetail{}},
		{"getIdeaHistory", "ACT-0613", "Read immutable Idea events", app.IdeaInput{}, app.Page[model.IdeaRevision]{}},
		{"getTicketIdeas", "ACT-0613", "Read currently authorized Idea sources", app.IDInput{}, app.Page[model.IdeaLink]{}},
		{"getIdeaLinks", "ACT-0613", "Read authorized Idea target links", app.IdeaInput{}, app.Page[model.IdeaLink]{}},
		{"getIdeaProposal", "ACT-0613", "Read an exact retained proposal and acceptance", app.IdeaProposalInput{}, app.IdeaProposalDetail{}},
		{"submitIdeaProposal", "ACT-0616", "Save an immutable human proposal", app.ProposeIdeaInput{}, model.IdeaProposal{}},
		{"acceptIdeaProposal", "ACT-0617", "Human acceptance creates or links Todo work atomically", app.AcceptIdeaInput{}, model.IdeaAcceptance{}},
		{"previewIdeaPlacement", "ACT-0618", "Preview the Idea publication boundary", app.IdeaControlInput{}, app.IdeaPlacementPreview{}},
		{"setIdeaPlacement", "ACT-0618", "Explicitly move an Idea and retained history", app.IdeaControlInput{}, model.Idea{}},
		{"setIdeaVisibility", "ACT-0619", "Archive, delete or restore an Idea", app.IdeaControlInput{}, model.Idea{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
