package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestSettingsPopupCannotBecomeNavigationDefault(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Preferences = map[string]model.Preferences{"admin": {Version: 3, Style: "paper", Mode: "light", DefaultView: "Settings", Recent: []model.RecentContext{{View: "Settings"}, {View: "Home"}}}}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	p := call(t, svc, token, "getViewPreferences", struct{}{}).(model.Preferences)
	if p.DefaultView != "Home" || p.Version != 3 || len(p.Recent) != 1 || p.Recent[0].View != "Home" {
		t.Fatal("legacy full-page settings escaped into navigation", p)
	}
	raw, _ := json.Marshal(app.ViewPreferencesInput{ExpectedVersion: p.Version, DefaultView: "Settings"})
	if _, err := svc.Call(context.Background(), token, "saveViewPreferences", raw); err == nil || app.ErrorResponse(err).Code != "invalid_view" {
		t.Fatal("popup saved as a destination", err)
	}
	saved := call(t, svc, token, "setAppearance", app.AppearanceInput{ExpectedVersion: p.Version, Style: "instrument", Mode: "light"}).(model.Preferences)
	if saved.DefaultView != "Home" || saved.Version != 4 {
		t.Fatal("ordinary preference save did not retain canonical navigation", saved)
	}
}
