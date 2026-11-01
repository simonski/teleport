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
		{"setExecutionMode", "ACT-0918", "Configure explicit off/demo execution", app.ExecutionModeInput{}, model.Settings{}},
		{"requestRefinement", "ACT-0301", "Request scoped refinement", app.VersionInput{}, model.Ticket{}},
		{"returnRefinementToTodo", "ACT-0403", "Return unowned queued refinement to human Todo", app.VersionInput{}, model.Ticket{}},
		{"runDemoStage", "ACT-0316", "Run one labelled deterministic demo stage", app.DemoStageInput{}, app.DemoStageResult{}},
		{"acceptRefinement", "ACT-0420", "Human accepts the exact current proposal", app.AcceptScopeInput{}, model.Ticket{}},
		{"answerQuestion", "ACT-0412", "Answer the exact pending structured question", app.AnswerInput{}, model.Ticket{}},
		{"approveTicket", "ACT-0306", "Require real delivery evidence for human code approval", app.ApprovalInput{}, model.Ticket{}},
		{"rejectTicket", "ACT-0307", "Choose an explicit implementation or scope revision", app.ApprovalInput{}, model.Ticket{}},
		{"setExecutionBudget", "ACT-0714", "Change finite attempts without resetting consumption", app.BudgetInput{}, model.Ticket{}},
		{"pauseTicket", "ACT-0313", "Pause admission and logical continuation", app.VersionInput{}, model.Ticket{}},
		{"resumeTicket", "ACT-0313", "Explicitly resume eligible held work", app.VersionInput{}, model.Ticket{}},
		{"setLaneCapacity", "ACT-0709", "Configure shared Refine and Doing capacity", app.CapacityInput{}, model.Project{}},
		{"setHandbrake", "ACT-0914", "Set system/project/stable-phase handbrake", app.HandbrakeInput{}, oneOf{model.Settings{}, model.Project{}}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
