package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestLastLocationSurvivesSignOutWithoutChangingPreferenceVersions(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Location", RequestID: "p"}).(model.Project)
	before := call(t, svc, token, "getViewPreferences", struct{}{}).(model.Preferences)
	contentBefore := call(t, svc, token, "getSession", struct{}{}).(map[string]any)["contentRevision"]
	call(t, svc, token, "saveCurrentLocation", map[string]any{"view": "Todos", "projectId": p.ID})
	if after := call(t, svc, token, "getSession", struct{}{}).(map[string]any)["contentRevision"]; after != contentBefore {
		t.Fatal("navigation-only persistence invalidated workspace content")
	}
	after := call(t, svc, token, "getViewPreferences", struct{}{}).(model.Preferences)
	if before.Version != after.Version || before.DefaultView != after.DefaultView {
		t.Fatal("remembered navigation overwrote personal preferences")
	}
	call(t, svc, token, "logout", struct{}{})
	result, err := svc.Call(context.Background(), "", "login", json.RawMessage(`{"username":"admin","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	token = result.(app.LoginResult).Token
	session := call(t, svc, token, "getSession", struct{}{}).(map[string]any)
	encoded, _ := json.Marshal(session["lastLocation"])
	var location model.RecentContext
	if json.Unmarshal(encoded, &location) != nil || location.View != "Todos" || location.ProjectID != p.ID {
		t.Fatal("sign-in lost last location", string(encoded))
	}
}

func TestLocationIsPrivateIdempotentAndDropsRevokedProject(t *testing.T) {
	svc, admin, db := fixtureWithStore(t)
	p := call(t, svc, admin, "createProject", app.CreateProjectInput{Name: "Visible", RequestID: "visible"}).(model.Project)
	other := call(t, svc, admin, "createProject", app.CreateProjectInput{Name: "Foreign", RequestID: "foreign"}).(model.Project)
	call(t, svc, admin, "saveCurrentLocation", app.CurrentLocationInput{View: "Graph", ProjectID: other.ID})
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Accounts["viewer"] = model.Account{ID: "viewer", Username: "viewer", PasswordHash: st.Accounts["admin"].PasswordHash}
		project := st.Projects[p.ID]
		project.Members["viewer"] = "Viewer"
		st.Projects[p.ID] = project
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Call(context.Background(), "", "login", json.RawMessage(`{"username":"viewer","password":"password"}`))
	if err != nil {
		t.Fatal(err)
	}
	token := result.(app.LoginResult).Token
	if _, ok := call(t, svc, token, "getSession", struct{}{}).(map[string]any)["lastLocation"]; ok {
		t.Fatal("another account inherited a location")
	}
	input := app.CurrentLocationInput{View: "Todos", ProjectID: p.ID}
	call(t, svc, token, "saveCurrentLocation", input)
	before := call(t, svc, token, "getSession", struct{}{}).(map[string]any)["revision"]
	call(t, svc, token, "saveCurrentLocation", input)
	if after := call(t, svc, token, "getSession", struct{}{}).(map[string]any)["revision"]; before != after {
		t.Fatal("identical navigation was not idempotent")
	}
	raw, _ := json.Marshal(app.CurrentLocationInput{View: "Board", ProjectID: other.ID})
	if _, err = svc.Call(context.Background(), token, "saveCurrentLocation", raw); err == nil {
		t.Fatal("foreign project location accepted")
	}
	if err = db.Update(context.Background(), func(st *model.State) (bool, error) {
		project := st.Projects[p.ID]
		delete(project.Members, "viewer")
		st.Projects[p.ID] = project
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := call(t, svc, token, "getSession", struct{}{}).(map[string]any)["lastLocation"]; ok {
		t.Fatal("revoked project context was exposed")
	}
}
