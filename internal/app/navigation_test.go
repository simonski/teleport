package app_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestNavigationLayoutPreservesOldClientsAndVersionFences(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "getViewPreferences", struct{}{}).(model.Preferences)
	if len(p.NavigationOrder) != 12 || p.NavigationOrder[4] != "Board" || p.NavigationPosition != "top" || p.NavigationCollapsed {
		t.Fatal("invalid new-account layout", p)
	}
	order := append([]string{}, p.NavigationOrder...)
	order[0], order[1] = order[1], order[0]
	left, collapsed := "left", true
	in := app.ViewPreferencesInput{ExpectedVersion: p.Version, DefaultView: "Home", NavigationOrder: &order, NavigationPosition: &left, NavigationCollapsed: &collapsed}
	saved := call(t, svc, token, "saveViewPreferences", in).(model.Preferences)
	in.ExpectedVersion = saved.Version
	noop := call(t, svc, token, "saveViewPreferences", in).(model.Preferences)
	if noop.Version != saved.Version {
		t.Fatal("identical layout created a revision")
	}
	oldClient := app.ViewPreferencesInput{ExpectedVersion: saved.Version, DefaultView: "Board"}
	preserved := call(t, svc, token, "saveViewPreferences", oldClient).(model.Preferences)
	if !reflect.DeepEqual(preserved.NavigationOrder, order) || preserved.NavigationPosition != "left" || !preserved.NavigationCollapsed {
		t.Fatal("old client reset navigation", preserved)
	}
	denied := func(in app.ViewPreferencesInput, code string) {
		t.Helper()
		raw, _ := json.Marshal(in)
		if _, err := svc.Call(context.Background(), token, "saveViewPreferences", raw); err == nil || app.ErrorResponse(err).Code != code {
			t.Fatal("invalid layout accepted", code, err)
		}
	}
	denied(in, "preference_conflict")
	in.ExpectedVersion = preserved.Version
	order[1] = order[0]
	denied(in, "invalid_navigation")
	order[1] = "System"
	denied(in, "invalid_navigation")
	in.NavigationOrder = nil
	left = "bottom"
	denied(in, "invalid_navigation")
	current := call(t, svc, token, "getViewPreferences", struct{}{}).(model.Preferences)
	if !reflect.DeepEqual(current, preserved) {
		t.Fatal("invalid write had partial effects", current)
	}
}
