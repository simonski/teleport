package server

import (
	"testing"

	"teleport/internal/app"
)

func TestProjectSummarySchemasDescribeUnavailableValues(t *testing.T) {
	properties := schema(app.ProjectSummary{})["properties"].(map[string]any)
	for _, field := range []string{"tokens", "agents", "lastActive"} {
		options, ok := properties[field].(map[string]any)["anyOf"].([]any)
		if !ok || len(options) != 2 || options[1].(map[string]any)["type"] != "null" {
			t.Fatal("nullable measurement rejected by generated schema", field, properties[field])
		}
	}
	input := schema(app.SaveProjectInput{})["properties"].(map[string]any)
	if input["description"].(map[string]any)["anyOf"] == nil {
		t.Fatal("metadata schema does not permit omission/null-preserve input")
	}
	if declaration, ok := contracts["listProjectSummaries"]; !ok || declaration.Output == nil {
		t.Fatal("project aggregate operation missing discovery contract")
	}
}
