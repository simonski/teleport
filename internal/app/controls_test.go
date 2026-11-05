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
	previewBody, _ := json.Marshal(app.TransferPreviewInput{ID: ticket.ID, DestinationProjectID: destination.ID})
	if _, err := svc.Call(context.Background(), ownerToken, "previewTicketTransfer", previewBody); err == nil {
		t.Fatal("preview exposed a transfer without ownership of both projects")
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

func TestTicketTransferPreviewIncludesParentAndHiddenChildren(t *testing.T) {
	svc, token := fixture(t)
	source := call(t, svc, token, "createProject", map[string]any{"name": "Transfer preview source", "requestId": "source"}).(model.Project)
	destination := call(t, svc, token, "createProject", map[string]any{"name": "Transfer preview destination", "requestId": "destination"}).(model.Project)
	unassigned := call(t, svc, token, "createTicket", map[string]any{"projectId": source.ID, "title": "Unassigned story", "agent": false, "requestId": "unassigned"}).(model.Ticket)
	single := call(t, svc, token, "previewTicketTransfer", app.TransferPreviewInput{ID: unassigned.ID, DestinationProjectID: destination.ID}).(app.TransferPreview)
	if single.IncludesParent || single.RootID != unassigned.ID || single.Total != 1 {
		t.Fatal("protected default was included in transfer", single)
	}
	epic := call(t, svc, token, "createTicket", map[string]any{"projectId": source.ID, "kind": "epic", "title": "Parent", "agent": false, "requestId": "epic"}).(model.Ticket)
	child := call(t, svc, token, "createTicket", map[string]any{"projectId": source.ID, "parentId": epic.ID, "title": "Story", "agent": false, "requestId": "child"}).(model.Ticket)
	hidden := call(t, svc, token, "createTicket", map[string]any{"projectId": source.ID, "parentId": epic.ID, "title": "Hidden", "agent": false, "requestId": "hidden"}).(model.Ticket)
	call(t, svc, token, "setTicketVisibility", map[string]any{"id": hidden.ID, "expectedVersion": hidden.Version, "archived": true})
	preview := call(t, svc, token, "previewTicketTransfer", app.TransferPreviewInput{ID: child.ID, DestinationProjectID: destination.ID}).(app.TransferPreview)
	if !preview.IncludesParent || preview.RootID != epic.ID || preview.Stories != 2 || preview.Total != 3 || preview.Hidden != 1 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	call(t, svc, token, "updateTicket", map[string]any{"id": child.ID, "expectedVersion": child.Version, "title": "Changed"})
	input := app.TransferInput{ID: preview.RootID, ExpectedVersion: preview.RootVersion, DestinationProjectID: destination.ID, ExpectedRevision: &preview.Revision}
	body, _ := json.Marshal(input)
	if _, err := svc.Call(context.Background(), token, "transferTicket", body); err == nil {
		t.Fatal("stale preview allowed transfer")
	}
	preview = call(t, svc, token, "previewTicketTransfer", app.TransferPreviewInput{ID: child.ID, DestinationProjectID: destination.ID}).(app.TransferPreview)
	call(t, svc, token, "transferTicket", app.TransferInput{ID: preview.RootID, ExpectedVersion: preview.RootVersion, DestinationProjectID: destination.ID, ExpectedRevision: &preview.Revision})
	for _, id := range []int64{epic.ID, child.ID, hidden.ID} {
		if call(t, svc, token, "getTicket", app.IDInput{ID: id}).(model.Ticket).ProjectID != destination.ID {
			t.Fatal("subtree did not move together")
		}
	}
}
