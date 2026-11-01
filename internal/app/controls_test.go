package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestSubtreeCancellationReopenAndSelectedVisibility(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Controls", "key": "controls", "requestId": "p"}).(model.Project)
	epic := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "kind": "epic", "title": "Shipment", "agent": false, "requestId": "e"}).(model.Ticket)
	child := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "parentId": epic.ID, "title": "Case", "agent": false, "requestId": "c"}).(model.Ticket)
	call(t, svc, token, "cancelTicket", map[string]any{"id": epic.ID, "expectedVersion": epic.Version, "reason": "Paused shipment"})
	epic = call(t, svc, token, "getTicket", map[string]any{"id": epic.ID}).(model.Ticket)
	call(t, svc, token, "reopenTicket", map[string]any{"id": epic.ID, "expectedVersion": epic.Version})
	child = call(t, svc, token, "getTicket", map[string]any{"id": child.ID}).(model.Ticket)
	if child.Status != "Cancelled" {
		t.Fatal("container reopen revived a child")
	}
	epic = call(t, svc, token, "getTicket", map[string]any{"id": epic.ID}).(model.Ticket)
	call(t, svc, token, "setTicketVisibility", map[string]any{"id": epic.ID, "expectedVersion": epic.Version, "archived": true})
	page := call(t, svc, token, "listTickets", app.ListInput{ProjectID: p.ID}).(app.Page[model.Ticket])
	found := false
	for _, v := range page.Items {
		if v.ID == child.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("hiding parent hid an unselected child")
	}
}

func TestTransferRequiresCurrentOwnershipOfBothProjects(t *testing.T) {
	svc, admin, db := fixtureWithStore(t)
	source := call(t, svc, admin, "createProject", map[string]any{"name": "Source", "key": "source", "requestId": "s"}).(model.Project)
	destination := call(t, svc, admin, "createProject", map[string]any{"name": "Destination", "key": "destination", "requestId": "d"}).(model.Project)
	ticket := call(t, svc, admin, "createTicket", map[string]any{"projectId": source.ID, "title": "Retained ticket", "requestId": "t"}).(model.Ticket)
	ownerToken := "test-only-owner-session"
	hashed := sha256.Sum256([]byte(ownerToken))
	setDestinationRole := func(destinationRole string) {
		t.Helper()
		if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
			st.Accounts["owner"] = model.Account{ID: "owner", Username: "owner"}
			st.Sessions[hex.EncodeToString(hashed[:])] = model.Session{AccountID: "owner", Expires: time.Now().Add(time.Hour)}
			s := st.Projects[source.ID]
			s.Members["owner"] = "Maintainer"
			st.Projects[source.ID] = s
			d := st.Projects[destination.ID]
			d.Members["owner"] = destinationRole
			st.Projects[destination.ID] = d
			return true, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	setDestinationRole("Contributor")
	input := map[string]any{"id": ticket.ID, "expectedVersion": ticket.Version, "destinationProjectId": destination.ID}
	body, _ := json.Marshal(input)
	if _, err := svc.Call(context.Background(), ownerToken, "transferTicket", body); err == nil {
		t.Fatal("one-project ownership allowed a transfer")
	}
	current := call(t, svc, admin, "getTicket", map[string]any{"id": ticket.ID}).(model.Ticket)
	if current.ProjectID != source.ID || current.Version != ticket.Version {
		t.Fatal("forbidden transfer partially mutated work")
	}
	setDestinationRole("Maintainer")
	current = call(t, svc, ownerToken, "transferTicket", input).(model.Ticket)
	if current.ProjectID != destination.ID {
		t.Fatal("dual owner transfer failed")
	}
}

func TestDependenciesAndTransfersAreAtomic(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Source", "key": "source", "requestId": "source"}).(model.Project)
	destination := call(t, svc, token, "createProject", map[string]any{"name": "Destination", "key": "destination", "requestId": "destination"}).(model.Project)
	a := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "A", "requestId": "a"}).(model.Ticket)
	b := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "B", "requestId": "b"}).(model.Ticket)
	a = call(t, svc, token, "setDependencies", map[string]any{"id": a.ID, "expectedVersion": a.Version, "dependencies": []int64{b.ID}}).(model.Ticket)
	for _, op := range []struct {
		name  string
		input any
	}{{"setDependencies", map[string]any{"id": b.ID, "expectedVersion": b.Version, "dependencies": []int64{a.ID}}}, {"transferTicket", map[string]any{"id": b.ID, "expectedVersion": b.Version, "destinationProjectId": destination.ID}}} {
		body, _ := json.Marshal(op.input)
		if _, err := svc.Call(context.Background(), token, op.name, body); err == nil {
			t.Fatalf("%s violated dependency graph", op.name)
		}
	}
	a = call(t, svc, token, "setDependencies", map[string]any{"id": a.ID, "expectedVersion": a.Version, "dependencies": []int64{}}).(model.Ticket)
	b = call(t, svc, token, "transferTicket", map[string]any{"id": b.ID, "expectedVersion": b.Version, "destinationProjectId": destination.ID}).(model.Ticket)
	if b.ProjectID != destination.ID || b.ParentID != destination.UnassignedEpic {
		t.Fatal("transfer failed to use destination defaults")
	}
}
