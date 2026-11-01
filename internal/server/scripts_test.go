package server

import (
	"encoding/json"
	"reflect"
	"testing"

	"teleport/internal/app"
)

func TestScriptBindingsMatchPublishedCanonicalSchemas(t *testing.T) {
	for _, binding := range app.ScriptBindings() {
		declaration, ok := contracts[binding.Operation]
		if !ok {
			t.Fatal("script binding has no canonical schema", binding.Operation)
		}
		if schema(declaration.Input)["type"] != "object" {
			t.Fatal("script input drift", binding.Operation)
		}
		registered := false
		for _, op := range Operations {
			if op.Name == binding.Operation && op.Action != "" {
				registered = true
				break
			}
		}
		if !registered {
			t.Fatal("script binding has no permanent action", binding.Operation)
		}
	}
	if got := schemaType(reflect.TypeFor[json.RawMessage]()); got["type"] == "array" {
		t.Fatal("JSON values advertised as byte arrays")
	}
}
