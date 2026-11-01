package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestManagedContextIsProjectScopedReadOnlyAndWithdrawn(t *testing.T) {
	svc, human, db := fixtureWithStore(t)
	p := call(t, svc, human, "createProject", app.CreateProjectInput{Name: "Context", RequestID: "context"}).(model.Project)
	other := call(t, svc, human, "createProject", app.CreateProjectInput{Name: "Other", RequestID: "other"}).(model.Project)
	token := strings.Repeat("e", 64)
	hash := sha256.Sum256([]byte(token))
	err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		project := st.Projects[p.ID]
		project.Repository = &model.RepositoryBinding{Version: 1}
		st.Projects[p.ID] = project
		st.LocalSessions = map[string]model.LocalSession{"H1": {ID: "H1", Creator: "admin", ProjectID: p.ID, State: "running", BindingVersion: 1, CredentialHash: hex.EncodeToString(hash[:]), CredentialExpires: time.Now().Add(time.Hour)}}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"getSession", "logout", "getLocalHarnesses", "listSessions", "approveTicket", "configureLocalRepository", "createProject", "saveProject", "listProjectSummaries", "login"} {
		if _, err := svc.Call(context.Background(), token, operation, json.RawMessage(`{}`)); err == nil {
			t.Fatal("context escaped to", operation)
		}
	}
	projects := call(t, svc, token, "listProjects", struct{}{}).(app.Page[model.Project])
	if len(projects.Items) != 1 || projects.Items[0].ID != p.ID {
		t.Fatal("context disclosed another project", other.ID, projects)
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		row := st.LocalSessions["H1"]
		row.StopRequested = true
		st.LocalSessions["H1"] = row
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Call(context.Background(), token, "listProjects", json.RawMessage(`{}`)); err == nil {
		t.Fatal("withdrawn credential remained valid")
	}
}
