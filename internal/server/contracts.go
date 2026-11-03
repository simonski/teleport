package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

type contract struct{ Input, Output any }

var contracts = map[string]contract{
	"login": {struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}, app.LoginResult{}},
	"getSession": {struct{}{}, struct {
		Account         model.Account        `json:"account"`
		InstallationID  string               `json:"installationId"`
		Revision        int64                `json:"revision"`
		ContentRevision int64                `json:"contentRevision"`
		Settings        model.Settings       `json:"settings"`
		Preferences     model.Preferences    `json:"preferences"`
		WelcomeSetup    model.WelcomeSetup   `json:"welcomeSetup"`
		LastLocation    *model.RecentContext `json:"lastLocation,omitempty"`
	}{}},
	"logout": {struct{}{}, struct {
		SignedOut bool `json:"signedOut"`
	}{}},
	"listWorkflows": {struct{}{}, struct {
		Items      []model.Workflow `json:"items"`
		NextCursor string           `json:"nextCursor"`
	}{}},
}

type oneOf []any

// OperationInputSchema exposes the same shipped request contract to offline CLI discovery.
func OperationInputSchema(name string) (map[string]any, bool) {
	definition, ok := contracts[name]
	if !ok {
		return nil, false
	}
	return schema(definition.Input), true
}

func schema(value any) map[string]any {
	if alternatives, ok := value.(oneOf); ok {
		items := []any{}
		for _, item := range alternatives {
			items = append(items, schema(item))
		}
		return map[string]any{"oneOf": items}
	}
	return schemaType(reflect.TypeOf(value))
}
func schemaType(t reflect.Type) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	if t.Kind() == reflect.Pointer {
		return map[string]any{"anyOf": []any{schemaType(t.Elem()), map[string]any{"type": "null"}}}
	}
	if t == reflect.TypeFor[time.Time]() {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t == reflect.TypeFor[json.RawMessage]() {
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || f.Name == "PasswordHash" {
				continue
			}
			tag := f.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			properties[name] = schemaType(f.Type)
			if !strings.Contains(tag, "omitempty") {
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schemaType(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaType(t.Elem())}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	default:
		return map[string]any{}
	}
}
