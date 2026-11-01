package server

import (
	"teleport/internal/app"
	"teleport/internal/model"
)

func init() {
	contracts["listWorkflows"] = contract{app.WorkflowListInput{}, app.Page[model.Workflow]{}}
	entries := []struct {
		name, action, summary string
		input, output         any
	}{
		{"getWorkflow", "ACT-0701", "Read an exact workflow definition", app.WorkflowKeyInput{}, model.Workflow{}},
		{"copyWorkflow", "ACT-0703", "Copy a workflow into a private draft", app.CopyWorkflowInput{}, model.Workflow{}},
		{"draftWorkflow", "ACT-0702", "Start the next draft of a published definition", app.WorkflowKeyInput{}, model.Workflow{}},
		{"saveWorkflow", "ACT-0702", "Validate and save an expected-version draft", app.SaveWorkflowInput{}, model.Workflow{}},
		{"publishWorkflow", "ACT-0720", "Publish an exact saved draft without selecting it", app.PublishWorkflowInput{}, model.Workflow{}},
		{"deleteWorkflow", "ACT-0705", "Delete an exact current draft", app.DeleteWorkflowInput{}, struct {
			Deleted bool `json:"deleted"`
		}{}},
		{"activateWorkflowVersion", "ACT-0715", "Publish and select a workflow for new project work", app.ActivateWorkflowInput{}, model.Project{}},
		{"previewWorkflowOutcome", "ACT-0718", "Preview declared routing without execution", app.OutcomePreviewInput{}, app.RoutePreview{}},
		{"previewWorkflowMigration", "ACT-0716", "Preview stopped same-role pin migration", app.MigrationInput{}, app.MigrationResult{}},
		{"migrateWorkflow", "ACT-0716", "Atomically migrate stopped work to same-role phases", app.MigrationInput{}, app.MigrationResult{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
