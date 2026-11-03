package app

import (
	"encoding/json"
	"testing"

	"teleport/internal/model"
)

func TestIdeaImportOriginIsImmutableAndRetainsLegacyRetry(t *testing.T) {
	state := &model.State{Receipts: map[string]model.Receipt{}}
	actor := model.Account{ID: "author"}
	call := func(input SaveIdeaInput) (model.Idea, error) {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		value, _, err := ideaOperations(state, actor, "saveIdea", raw)
		if err != nil {
			return model.Idea{}, err
		}
		return value.(model.Idea), nil
	}
	created, err := call(SaveIdeaInput{Title: "Notes", Text: "Text", Origin: "import", RequestID: "new"})
	if err != nil || created.Origin != "import" {
		t.Fatalf("import origin: %+v %v", created, err)
	}
	updated, err := call(SaveIdeaInput{ID: created.ID, ExpectedVersion: created.Version, Title: "Edited", Text: "Text"})
	if err != nil || updated.Origin != "import" {
		t.Fatalf("edit changed origin: %+v %v", updated, err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var restored model.State
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.Ideas[created.ID].Origin != "import" {
		t.Fatalf("origin did not survive storage round trip: %v", err)
	}
	if _, err = call(SaveIdeaInput{ID: created.ID, ExpectedVersion: updated.Version, Title: "Edited", Origin: "import"}); err == nil {
		t.Fatal("origin update accepted")
	}
	legacy, err := call(SaveIdeaInput{Title: "Older", Text: "Notes", RequestID: "old"})
	if err != nil {
		t.Fatal(err)
	}
	retried, err := call(SaveIdeaInput{Title: "Older", Text: "Notes", Origin: "import", RequestID: "old"})
	if err != nil || retried.ID != legacy.ID || retried.Origin != "" {
		t.Fatalf("legacy retry changed: %+v %v", retried, err)
	}
	if _, err = call(SaveIdeaInput{Title: "Changed", Text: "Notes", Origin: "import", RequestID: "old"}); err == nil {
		t.Fatal("conflicting retry accepted")
	}
	if _, err = call(SaveIdeaInput{Title: "Invalid", Origin: "other", RequestID: "invalid"}); err == nil {
		t.Fatal("invalid origin accepted")
	}
}
