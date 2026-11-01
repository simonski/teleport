package server

import (
	"teleport/internal/app"
	"teleport/internal/model"
)

func init() {
	entries := []struct {
		name, summary string
		input, output any
	}{
		{"getWelcomeSetup", "Read private Welcome progress and local setup capabilities", struct{}{}, app.WelcomeSetupResult{}},
		{"checkProjectAvailability", "Check project name/prefix availability without disclosing projects", app.ProjectAvailabilityInput{}, app.ProjectAvailability{}},
		{"saveWelcomeSetup", "Save bounded private Welcome progress without starting execution", app.SaveWelcomeSetupInput{}, model.WelcomeSetup{}},
		{"completeWelcomeSetup", "Complete the reviewed setup without enabling execution", app.CompleteWelcomeSetupInput{}, model.WelcomeSetup{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, "ACT-1220", e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
