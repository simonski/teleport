package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestWorkflowPublicationRetrySelectionAndImmutableVersions(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Publish", RequestID: "p"}).(model.Project)
	before := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "Before publication", "agent": false, "requestId": "before"}).(model.Ticket)
	draft := call(t, svc, token, "copyWorkflow", app.CopyWorkflowInput{SourceKey: "human@1", ProjectID: p.ID, Title: "Published separately", RequestID: "flow"}).(model.Workflow)
	in := app.PublishWorkflowInput{WorkflowKey: draft.ID + "@draft", ExpectedRevision: draft.DraftRevision, RequestID: "publish-1"}
	first := call(t, svc, token, "publishWorkflow", in).(model.Workflow)
	if !first.Published || first.Version != 1 || first.DraftRevision != 0 {
		t.Fatal("publication did not freeze v-1", first)
	}
	retry := call(t, svc, token, "publishWorkflow", in).(model.Workflow)
	if !reflect.DeepEqual(first, retry) {
		t.Fatal("publication replay changed immutable result")
	}
	projects := call(t, svc, token, "listProjects", app.ListInput{}).(app.Page[model.Project])
	if !reflect.DeepEqual(projects.Items[0], p) {
		t.Fatal("publication selected a project or changed configuration")
	}
	stillOld := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "After publication only", "agent": false, "requestId": "still-old"}).(model.Ticket)
	if stillOld.Workflow != before.Workflow {
		t.Fatal("publication implicitly activated a workflow")
	}
	next := call(t, svc, token, "draftWorkflow", app.WorkflowKeyInput{WorkflowKey: draft.ID + "@1"}).(model.Workflow)
	if next.Version != 2 || next.Published {
		t.Fatal("next draft has wrong version", next)
	}
	next.Phases[0].Title = "New incoming work"
	next = call(t, svc, token, "saveWorkflow", app.SaveWorkflowInput{Definition: next, ExpectedRevision: next.DraftRevision}).(model.Workflow)
	denied := func(input app.PublishWorkflowInput, code string) {
		t.Helper()
		raw, _ := json.Marshal(input)
		if _, err := svc.Call(context.Background(), token, "publishWorkflow", raw); err == nil || app.ErrorResponse(err).Code != code {
			t.Fatal("incorrect publication error", code, err)
		}
	}
	denied(app.PublishWorkflowInput{WorkflowKey: in.WorkflowKey, ExpectedRevision: next.DraftRevision, RequestID: in.RequestID}, "idempotency_conflict")
	denied(app.PublishWorkflowInput{WorkflowKey: in.WorkflowKey, ExpectedRevision: next.DraftRevision - 1, RequestID: "stale"}, "workflow_conflict")
	second := call(t, svc, token, "publishWorkflow", app.PublishWorkflowInput{WorkflowKey: in.WorkflowKey, ExpectedRevision: next.DraftRevision, RequestID: "publish-2"}).(model.Workflow)
	if second.Version != 2 || !second.Published {
		t.Fatal("second publication did not freeze next number")
	}
	retained := call(t, svc, token, "getWorkflow", app.WorkflowKeyInput{WorkflowKey: draft.ID + "@1"}).(model.Workflow)
	if !reflect.DeepEqual(retained, first) {
		t.Fatal("new version rewrote old publication")
	}
	p = call(t, svc, token, "activateWorkflowVersion", app.ActivateWorkflowInput{WorkflowKey: fmt.Sprintf("%s@%d", second.ID, second.Version), ProjectID: p.ID, ExpectedProjectVersion: p.Version}).(model.Project)
	after := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "After explicit selection", "agent": false, "requestId": "after"}).(model.Ticket)
	if after.Workflow != p.HumanWorkflow || before.Workflow != "human@1" {
		t.Fatal("explicit selection missed new work or changed old pins")
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		a := st.Accounts["admin"]
		a.Administrator = false
		st.Accounts[a.ID] = a
		project := st.Projects[p.ID]
		project.Members[a.ID] = "Viewer"
		st.Projects[p.ID] = project
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	denied(in, "forbidden")
}
