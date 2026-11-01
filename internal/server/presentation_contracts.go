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
		{"getGraph", "ACT-1201", "Read a bounded authorized ticket graph", app.GraphInput{}, app.Graph{}},
		{"saveGraphLayout", "ACT-1202", "Save separate shared-project or private All projects coordinates", app.GraphLayoutInput{}, model.GraphLayout{}},
		{"getAppearanceStyles", "ACT-0713", "Discover bundled browser styles and modes", struct{}{}, app.AppearanceStyles{}},
		{"getViewPreferences", "ACT-1205", "Read current account preferences with revalidated context", struct{}{}, model.Preferences{}},
		{"setAppearance", "ACT-0713", "Save only the current account's appearance", app.AppearanceInput{}, model.Preferences{}},
		{"saveViewPreferences", "ACT-1205", "Save versioned personal navigation defaults", app.ViewPreferencesInput{}, model.Preferences{}},
	}
	for _, e := range entries {
		Operations = append(Operations, Operation{e.name, e.action, e.summary})
		contracts[e.name] = contract{e.input, e.output}
	}
}
