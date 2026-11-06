package app_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestPersonalProjectProvisioningAndAdministratorPrivacy(t *testing.T) {
	svc, _, db := fixtureWithStore(t)
	initial, err := db.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Projects) != 1 {
		t.Fatalf("bootstrap account lacks its personal project: %#v", initial.Projects)
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		a := st.Accounts["admin"]
		st.Accounts["other"] = model.Account{ID: "other", Username: "other", PasswordHash: a.PasswordHash, Administrator: true}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Initialize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	initialized, err := db.Read(context.Background())
	if err != nil || len(initialized.Projects) != 2 {
		t.Fatalf("existing account provisioning was not idempotent: %#v, %v", initialized, err)
	}
	login := func(name string) string {
		raw, _ := json.Marshal(map[string]string{"username": name, "password": "password"})
		v, err := svc.Call(context.Background(), "", "login", raw)
		if err != nil {
			t.Fatal(err)
		}
		return v.(app.LoginResult).Token
	}
	admin := login("admin")
	other := login("other")
	_ = login("other")
	projects := call(t, svc, other, "listProjects", app.ListInput{}).(app.Page[model.Project]).Items
	if len(projects) != 1 || projects[0].Name != "Personal" || projects[0].PersonalOwner != "other" {
		t.Fatalf("personal project not provisioned once: %#v", projects)
	}
	p := projects[0]
	story := call(t, svc, other, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Private story", RequestID: "private"}).(model.Ticket)
	idea := call(t, svc, other, "saveIdea", app.SaveIdeaInput{ProjectID: p.ID, Title: "Private idea", Text: "Private content", RequestID: "idea"}).(model.Idea)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		if st.LocalSessions == nil {
			st.LocalSessions = map[string]model.LocalSession{}
		}
		if st.ScriptRuns == nil {
			st.ScriptRuns = map[string]model.ScriptRun{}
		}
		st.LocalSessions["private-session"] = model.LocalSession{ID: "private-session", Number: 1, Creator: "other", ProjectID: p.ID, TicketID: story.ID, Harness: "codex", State: "exited", CreatedAt: time.Now().UTC()}
		st.ScriptRuns["private-run"] = model.ScriptRun{ID: "private-run", Number: 1, Creator: "other", Projects: []int64{p.ID}, Source: "Private script", State: "running"}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	adminProjects := call(t, svc, admin, "listProjects", app.ListInput{}).(app.Page[model.Project]).Items
	if len(adminProjects) != 1 || adminProjects[0].PersonalOwner != "admin" {
		t.Fatal("other personal project disclosed", adminProjects)
	}
	summaries := call(t, svc, admin, "listProjectSummaries", app.ProjectDirectoryInput{}).(app.Page[app.ProjectSummary]).Items
	if len(summaries) != 1 || summaries[0].ID == p.ID {
		t.Fatal("other personal project disclosed in summaries", summaries)
	}
	if ideas := call(t, svc, admin, "listIdeas", app.ListIdeasInput{}).(app.Page[model.Idea]); len(ideas.Items) != 0 {
		t.Fatal("other personal project's ideas disclosed", ideas)
	}
	if sessions := call(t, svc, admin, "listSessions", app.ListInput{}).(app.LocalSessionPage); len(sessions.Items) != 0 {
		t.Fatal("other personal project's sessions disclosed", sessions)
	}
	if runs := call(t, svc, admin, "listScriptRuns", app.ListInput{}).(app.ScriptPage); len(runs.Items) != 1 || runs.Items[0].ID != "private-run" {
		t.Fatal("administrator operational metadata missing", runs)
	}
	if usage := call(t, svc, admin, "listUsageBreakdown", app.UsageBreakdownInput{GroupBy: "project", Window: "all"}).(app.UsageBreakdownPage); len(usage.Items) != 0 {
		t.Fatal("other personal project's usage disclosed", usage)
	}
	for _, node := range call(t, svc, admin, "getGraph", app.GraphInput{}).(app.Graph).Nodes {
		if node.ProjectID == p.ID {
			t.Fatal("other personal project disclosed in all-project graph", node)
		}
	}
	for _, req := range []struct {
		op    string
		value any
	}{{"getTicket", app.IDInput{ID: story.ID}}, {"listTickets", app.ListInput{ProjectID: p.ID}}, {"getGraph", app.GraphInput{ProjectID: p.ID}}, {"getIdea", app.IdeaInput{ID: idea.ID}}, {"getIdeaHistory", app.IdeaInput{ID: idea.ID}}, {"getUsageAttribution", app.SessionIDInput{ID: "private-session"}}, {"getScriptRun", app.ScriptIDInput{ID: "private-run"}}, {"transferTicket", app.TransferInput{ID: story.ID, ExpectedVersion: story.Version, DestinationProjectID: adminProjects[0].ID}}, {"previewProjectRemoval", app.ProjectRemovalInput{ID: p.ID}}, {"configureLocalRepository", app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version, Repository: "/not-granted"}}} {
		raw, _ := json.Marshal(req.value)
		if _, err := svc.Call(context.Background(), admin, req.op, raw); err == nil {
			t.Fatalf("other administrator accessed personal project via %s", req.op)
		}
	}
	raw, _ := json.Marshal(app.ProjectRemovalInput{ID: p.ID})
	if _, err := svc.Call(context.Background(), other, "previewProjectRemoval", raw); err == nil {
		t.Fatal("personal project may be removed")
	}
}
