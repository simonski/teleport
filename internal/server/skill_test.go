package server

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	teleportskill "teleport/internal/skills/teleport"
)

func TestSkillEndpointMatchesStandaloneBinaryArtifact(t *testing.T) {
	response := httptest.NewRecorder()
	New(nil, "test").ServeHTTP(response, httptest.NewRequest("GET", "http://localhost/skill.md", nil))
	if response.Code != 200 || response.Body.String() != teleportskill.Markdown {
		t.Fatalf("status=%d skill does not match the embedded artifact", response.Code)
	}
	if response.Header().Get("Content-Type") != "text/markdown; charset=utf-8" {
		t.Fatal("skill is not served as Markdown")
	}
}

func TestSkillExamplesUseRegisteredOperationsAndCurrentRequestSchemas(t *testing.T) {
	commands := regexp.MustCompile(`(?m)^tk ([A-Za-z]+)(?: -{1,2}data '([^']+)')?$`).FindAllStringSubmatch(teleportskill.Markdown, -1)
	checked := 0
	for _, command := range commands {
		name, data := command[1], command[2]
		if name == "skill" || name == "version" || name == "tools" || name == "spec" || name == "help" {
			continue
		}
		contract, ok := contracts[name]
		if !ok {
			t.Fatalf("skill advertises an unavailable operation: %s", name)
		}
		if data == "" {
			data = "{}"
		}
		input := reflect.New(reflect.TypeOf(contract.Input)).Interface()
		decoder := json.NewDecoder(strings.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(input); err != nil {
			t.Fatalf("%s example violates its request contract: %v", name, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(data), &fields); err != nil {
			t.Fatal(err)
		}
		for _, required := range schema(contract.Input)["required"].([]string) {
			if _, present := fields[required]; !present {
				t.Errorf("%s example omits required field %s", name, required)
			}
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no executable API examples were verified")
	}
}
