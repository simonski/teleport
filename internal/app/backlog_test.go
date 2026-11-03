package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestBacklogOrderAndParentMoveAreAtomicAndPreservePriority(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Backlog", RequestID: "p"}).(model.Project)
	goal := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Kind: "goal", Title: "Goal", RequestID: "g"}).(model.Ticket)
	epics := []model.Ticket{}
	for index := 0; index < 2; index++ {
		epics = append(epics, call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, ParentID: goal.ID, Kind: "epic", Title: fmt.Sprint("Epic", index), RequestID: fmt.Sprint("e", index)}).(model.Ticket))
	}
	human := false
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, ParentID: epics[0].ID, Title: "Story", Agent: &human, RequestID: "s"}).(model.Ticket)
	keys := []string{fmt.Sprintf("ticket:%d", ticket.ID), fmt.Sprintf("ticket:%d", epics[1].ID)}
	order := call(t, svc, token, "saveBacklogOrder", app.SaveBacklogInput{ProjectID: p.ID, Keys: keys}).(model.BacklogOrder)
	if order.Version != 1 || call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket).Version != ticket.Version {
		t.Fatal("presentation order mutated work")
	}
	parent := call(t, svc, token, "getTicket", app.IDInput{ID: epics[1].ID}).(model.Ticket)
	in := app.SaveBacklogInput{ProjectID: p.ID, ExpectedVersion: order.Version, Keys: []string{keys[1], keys[0]}, Move: &app.BacklogMove{ID: ticket.ID, ExpectedVersion: ticket.Version, ParentID: parent.ID, ExpectedParentVersion: parent.Version - 1}}
	deny := func(input app.SaveBacklogInput, code string) {
		t.Helper()
		raw, _ := json.Marshal(input)
		if _, err := svc.Call(context.Background(), token, "saveBacklogOrder", raw); err == nil || app.ErrorResponse(err).Code != code {
			t.Fatal("unexpected backlog result", code, err)
		}
	}
	deny(in, "version_conflict")
	if actual := call(t, svc, token, "getBacklogOrder", app.BacklogOrderInput{ProjectID: p.ID}).(model.BacklogOrder); !reflect.DeepEqual(actual, order) {
		t.Fatal("failed move partially saved order", actual)
	}
	if actual := call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket); actual.ParentID != ticket.ParentID {
		t.Fatal("failed move partially changed hierarchy")
	}
	in.Move.ExpectedParentVersion = parent.Version
	saved := call(t, svc, token, "saveBacklogOrder", in).(model.BacklogOrder)
	moved := call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket)
	if moved.ParentID != parent.ID || moved.Priority != ticket.Priority || moved.Workflow != ticket.Workflow || saved.Version != 2 {
		t.Fatal("move altered unrelated execution state", moved, saved)
	}
	deny(in, "backlog_conflict")
	moved = call(t, svc, token, "transitionTicket", app.TransitionInput{ID: moved.ID, ExpectedVersion: moved.Version, Target: "Doing"}).(model.Ticket)
	in.ExpectedVersion = saved.Version
	in.Move.ExpectedVersion = moved.Version
	in.Move.ParentID = epics[0].ID
	deny(in, "backlog_locked")
	if actual := call(t, svc, token, "getBacklogOrder", app.BacklogOrderInput{ProjectID: p.ID}).(model.BacklogOrder); !reflect.DeepEqual(actual, saved) {
		t.Fatal("locked move partially saved order")
	}
}

func TestBacklogRejectsInvalidParentsAndOrderKeys(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "One", RequestID: "one"}).(model.Project)
	q := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Two", RequestID: "two"}).(model.Project)
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Story", RequestID: "story"}).(model.Ticket)
	parent := call(t, svc, token, "getTicket", app.IDInput{ID: q.UnassignedEpic}).(model.Ticket)
	key := fmt.Sprintf("ticket:%d", ticket.ID)
	for _, input := range []app.SaveBacklogInput{{ProjectID: p.ID, Keys: []string{key, key}}, {ProjectID: p.ID, Keys: []string{"idea:missing"}}, {ProjectID: p.ID, Keys: []string{key}, Move: &app.BacklogMove{ID: ticket.ID, ExpectedVersion: ticket.Version, ParentID: parent.ID, ExpectedParentVersion: parent.Version}}} {
		raw, _ := json.Marshal(input)
		if _, err := svc.Call(context.Background(), token, "saveBacklogOrder", raw); err == nil {
			t.Fatal("invalid backlog edit accepted")
		}
	}
	order := call(t, svc, token, "getBacklogOrder", app.BacklogOrderInput{ProjectID: p.ID}).(model.BacklogOrder)
	if order.Version != 0 || len(order.Keys) != 0 {
		t.Fatal("invalid edit had partial effects", order)
	}
}

func TestBacklogOrderIsPrivateAndRevalidatesCurrentRights(t *testing.T) {
	svc, admin, db := fixtureWithStore(t)
	p := call(t, svc, admin, "createProject", app.CreateProjectInput{Name: "Scoped", RequestID: "p"}).(model.Project)
	ticket := call(t, svc, admin, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Visible story", RequestID: "s"}).(model.Ticket)
	idea := call(t, svc, admin, "saveIdea", app.SaveIdeaInput{Title: "Private thought", Text: "Not shared", RequestID: "i"}).(model.Idea)
	token := "backlog-viewer"
	hash := sha256.Sum256([]byte(token))
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Accounts["reader"] = model.Account{ID: "reader", Username: "reader"}
		st.Sessions[hex.EncodeToString(hash[:])] = model.Session{AccountID: "reader", Expires: time.Now().Add(time.Hour)}
		project := st.Projects[p.ID]
		project.Members["reader"] = "Viewer"
		st.Projects[p.ID] = project
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("ticket:%d", ticket.ID)
	order := call(t, svc, token, "saveBacklogOrder", app.SaveBacklogInput{Keys: []string{key}}).(model.BacklogOrder)
	if own := call(t, svc, admin, "getBacklogOrder", app.BacklogOrderInput{}).(model.BacklogOrder); own.Version != 0 || len(own.Keys) != 0 {
		t.Fatal("private order leaked across accounts")
	}
	for _, input := range []app.SaveBacklogInput{{ExpectedVersion: order.Version, Keys: []string{idea.ID}}, {ExpectedVersion: order.Version, Keys: []string{key}, Move: &app.BacklogMove{ID: ticket.ID, ExpectedVersion: ticket.Version, ParentID: p.UnassignedEpic}}} {
		raw, _ := json.Marshal(input)
		if _, err := svc.Call(context.Background(), token, "saveBacklogOrder", raw); err == nil {
			t.Fatal("private idea or work mutation escaped Viewer rights")
		}
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		project := st.Projects[p.ID]
		delete(project.Members, "reader")
		st.Projects[p.ID] = project
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if hidden := call(t, svc, token, "getBacklogOrder", app.BacklogOrderInput{}).(model.BacklogOrder); hidden.Version != order.Version || len(hidden.Keys) != 0 {
		t.Fatal("revoked project identifiers leaked", hidden)
	}
}
