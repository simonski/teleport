package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
	"teleport/internal/store"
)

func fixture(t *testing.T) (*app.Service, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "database.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err = app.Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	svc := app.New(db)
	result, err := svc.Call(context.Background(), "", "login", json.RawMessage(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	return svc, result.(app.LoginResult).Token
}

func TestNoOpAndConcurrentExpectedVersionUpdates(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Versions", "key": "versions", "requestId": "project-versions"}).(model.Project)
	ticket := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "Original", "agent": false, "requestId": "task-versions"}).(model.Ticket)
	unchanged := call(t, svc, token, "updateTicket", map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version, "title": "Original"}).(model.Ticket)
	if unchanged.Version != ticket.Version {
		t.Fatal("no-op created a revision")
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input, _ := json.Marshal(map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version, "title": fmt.Sprintf("Revision %d", i)})
			if _, err := svc.Call(context.Background(), token, "updateTicket", input); err == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("concurrent updates committed %d times", successes.Load())
	}
	current := call(t, svc, token, "getTicket", map[string]any{"id": ticket.ID}).(model.Ticket)
	if current.Version != ticket.Version+1 {
		t.Fatal("lost version monotonicity")
	}
}
func call(t *testing.T, svc *app.Service, token, op string, input any) any {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Call(context.Background(), token, op, body)
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	return result
}

func TestHumanTicketLifecycleVersionsAndHierarchy(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Flight", "key": "flight", "requestId": "project-1"}).(model.Project)
	retry := call(t, svc, token, "createProject", map[string]any{"name": "Flight", "key": "flight", "requestId": "project-1"}).(model.Project)
	if p.ID != retry.ID {
		t.Fatal("retried project created duplicates")
	}
	input := map[string]any{"projectId": p.ID, "title": "Pack the flight case", "agent": false, "requestId": "task-1"}
	ticket := call(t, svc, token, "createTicket", input).(model.Ticket)
	if ticket.ParentID != p.UnassignedEpic || ticket.Status != "Todo" || ticket.Agent {
		t.Fatal("invalid human/default hierarchy")
	}
	ticket = call(t, svc, token, "transitionTicket", map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version, "target": "Doing"}).(model.Ticket)
	body := json.RawMessage(fmt.Sprintf(`{"id":%d,"expectedVersion":%d,"title":"Forbidden edit"}`, ticket.ID, ticket.Version))
	if _, err := svc.Call(context.Background(), token, "updateTicket", body); err == nil {
		t.Fatal("non-Todo content edit succeeded")
	}
	ticket = call(t, svc, token, "transitionTicket", map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version, "target": "Done", "confirm": true}).(model.Ticket)
	epic := call(t, svc, token, "getTicket", map[string]any{"id": p.UnassignedEpic}).(model.Ticket)
	if epic.Status != "Done" || epic.Progress != 100 {
		t.Fatal("parent completion not derived")
	}
	page := call(t, svc, token, "listTicketHistory", map[string]any{"id": ticket.ID}).(app.Page[model.Revision])
	if len(page.Items) != 3 || page.Items[0].Snapshot.Status != "Todo" {
		t.Fatal("immutable version history lost")
	}
	if _, err := svc.Call(context.Background(), token, "transitionTicket", json.RawMessage(fmt.Sprintf(`{"id":%d,"expectedVersion":1,"target":"Todo"}`, ticket.ID))); err == nil {
		t.Fatal("stale write succeeded")
	}
}
