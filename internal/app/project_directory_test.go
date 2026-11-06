package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestProjectDirectoryCountsHistoryAndMetadataVersions(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Directory", Description: "Initial description", RequestID: "p"}).(model.Project)
	other := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Other", RequestID: "other"}).(model.Project)
	for _, kind := range []string{"goal", "epic", "task"} {
		call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Kind: kind, Title: kind, RequestID: kind})
	}
	hidden := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Archived", RequestID: "hidden"}).(model.Ticket)
	call(t, svc, token, "setTicketVisibility", map[string]any{"id": hidden.ID, "expectedVersion": hidden.Version, "archived": true})
	page := call(t, svc, token, "listProjectSummaries", app.ProjectDirectoryInput{Limit: 1}).(app.Page[app.ProjectSummary])
	if len(page.Items) != 1 || page.Items[0].Name != "Personal" || page.NextCursor == "" {
		t.Fatal("missing personal project", page)
	}
	page = call(t, svc, token, "listProjectSummaries", app.ProjectDirectoryInput{Cursor: page.NextCursor, Limit: 1}).(app.Page[app.ProjectSummary])
	row := page.Items[0]
	if row.Tickets != 3 || row.Epics != 1 || row.Goals != 1 || row.Tokens != nil || row.UsageStatus != "unavailable" || row.Agents == nil || *row.Agents != 0 || row.LastActive == nil {
		t.Fatal("incorrect aggregate", row)
	}
	last := *row.LastActive
	next := call(t, svc, token, "listProjectSummaries", app.ProjectDirectoryInput{Cursor: page.NextCursor, Limit: 1}).(app.Page[app.ProjectSummary])
	if len(next.Items) != 1 || next.Items[0].ID != other.ID || next.Items[0].Tickets != 0 || next.Items[0].LastActive != nil || next.NextCursor != "" {
		t.Fatal("incorrect page or protected-default activity", next)
	}
	before := call(t, svc, token, "getTicketAgreements", app.IDInput{ID: hidden.ID}).(app.AgreementBasis)
	name, description := "Renamed directory", "Updated description"
	in := app.SaveProjectInput{ID: p.ID, ExpectedVersion: p.Version, Name: &name, Description: &description}
	saved := call(t, svc, token, "saveProject", in).(model.Project)
	if saved.Key != p.Key || saved.Version != p.Version+1 || saved.Description != description || !reflect.DeepEqual(saved.Members, p.Members) {
		t.Fatal("metadata changed unrelated configuration", saved)
	}
	deny := func(operation string, input any, code string) {
		t.Helper()
		raw, _ := json.Marshal(input)
		if _, err := svc.Call(context.Background(), token, operation, raw); err == nil || app.ErrorResponse(err).Code != code {
			t.Fatal("unexpected failure", operation, code, err)
		}
	}
	deny("saveProject", in, "version_conflict")
	in.ExpectedVersion = saved.Version
	noop := call(t, svc, token, "saveProject", in).(model.Project)
	if noop.Version != saved.Version {
		t.Fatal("no-op revised project")
	}
	name = "  OTHER "
	deny("saveProject", in, "project_name_exists")
	name, description = saved.Name, strings.Repeat("x", 20001)
	deny("saveProject", in, "invalid_project")
	deny("listProjectSummaries", app.ProjectDirectoryInput{Limit: 201}, "invalid_limit")
	deny("listProjectSummaries", app.ProjectDirectoryInput{Cursor: "!"}, "invalid_cursor")
	description = ""
	saved = call(t, svc, token, "saveProject", in).(model.Project)
	if saved.Description != "" {
		t.Fatal("empty description did not clear")
	}
	row = call(t, svc, token, "listProjectSummaries", app.ProjectDirectoryInput{Query: "Renamed"}).(app.Page[app.ProjectSummary]).Items[0]
	if !row.LastActive.Equal(last) || row.Description != "" || row.Name != saved.Name {
		t.Fatal("read or metadata edit changed work activity", row)
	}
	after := call(t, svc, token, "getTicketAgreements", app.IDInput{ID: hidden.ID}).(app.AgreementBasis)
	if before != after {
		t.Fatal("metadata changed agreement basis")
	}
}

func TestProjectDirectoryCurrentRightsAndUnverifiedPrivateSessions(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Allowed", RequestID: "allowed"}).(model.Project)
	call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Foreign", RequestID: "foreign"})
	viewer := "directory-viewer"
	hash := sha256.Sum256([]byte(viewer))
	stamp := time.Now().UTC().Truncate(time.Second)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Accounts["viewer"] = model.Account{ID: "viewer", Username: "viewer"}
		st.Sessions[hex.EncodeToString(hash[:])] = model.Session{AccountID: "viewer", Expires: time.Now().Add(time.Hour)}
		project := st.Projects[p.ID]
		project.Members["viewer"] = "Viewer"
		st.Projects[p.ID] = project
		st.LocalSessions = map[string]model.LocalSession{"H1": {ID: "H1", ProjectID: p.ID, Creator: "admin", State: "running", CreatedAt: stamp, CredentialHash: "private", LaunchID: "unverified"}}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	admin := call(t, svc, token, "listProjectSummaries", app.ProjectDirectoryInput{Query: "Allowed"}).(app.Page[app.ProjectSummary]).Items[0]
	if admin.Agents != nil || admin.LastActive == nil || !admin.LastActive.Equal(stamp) {
		t.Fatal("unverified running session counted", admin)
	}
	page := call(t, svc, viewer, "listProjectSummaries", app.ProjectDirectoryInput{}).(app.Page[app.ProjectSummary])
	if len(page.Items) != 1 || page.Items[0].ID != p.ID || page.Items[0].Agents == nil || *page.Items[0].Agents != 0 || page.Items[0].LastActive != nil {
		t.Fatal("private activity or foreign project leaked", page)
	}
	raw, _ := json.Marshal(app.SaveProjectInput{ID: p.ID, ExpectedVersion: p.Version})
	if _, err := svc.Call(context.Background(), viewer, "saveProject", raw); err == nil {
		t.Fatal("Viewer edited project metadata")
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		project := st.Projects[p.ID]
		delete(project.Members, "viewer")
		st.Projects[p.ID] = project
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	page = call(t, svc, viewer, "listProjectSummaries", app.ProjectDirectoryInput{}).(app.Page[app.ProjectSummary])
	if len(page.Items) != 0 {
		t.Fatal("revoked project remained visible")
	}
}
