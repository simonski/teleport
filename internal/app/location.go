package app

import (
	"encoding/json"
	"teleport/internal/model"
)

type CurrentLocationInput struct {
	View      string `json:"view"`
	ProjectID int64  `json:"projectId"`
}

func currentLocation(st *model.State, a model.Account) *model.RecentContext {
	location, ok := st.LastLocations[a.ID]
	location.View = canonicalView(location.View)
	if !ok || validRecent(st, a, location) != nil {
		return nil
	}
	return &location
}

func saveCurrentLocation(st *model.State, a model.Account, raw json.RawMessage) (any, bool, error) {
	if a.ServiceRunID != "" || a.ServiceSessionID != "" {
		return nil, false, failure(403, "human_required", "Only an authenticated account owner may save navigation")
	}
	var in CurrentLocationInput
	if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	location := model.RecentContext{View: in.View, ProjectID: in.ProjectID}
	location.View = canonicalView(location.View)
	if err := validRecent(st, a, location); err != nil {
		return nil, false, err
	}
	if previous, ok := st.LastLocations[a.ID]; ok && previous == location {
		return location, false, nil
	}
	if st.LastLocations == nil {
		st.LastLocations = map[string]model.RecentContext{}
	}
	st.LastLocations[a.ID] = location
	st.LocationWrites++
	return location, true, nil
}
