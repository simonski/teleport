package app

import (
	"teleport/internal/model"
	"testing"
)

func TestUsageAttributionSnapshotsHierarchyAndDelegation(t *testing.T) {
	st := &model.State{Projects: map[int64]model.Project{1: {ID: 1, Version: 4, Members: map[string]string{"u": "Viewer"}}, 2: {ID: 2}}, Tickets: map[int64]model.Ticket{1: {ID: 1, Kind: "goal", ProjectID: 1, Version: 2, Title: "Goal"}, 2: {ID: 2, Kind: "epic", ProjectID: 1, ParentID: 1, Version: 3, Title: "Epic"}, 3: {ID: 3, Kind: "task", ProjectID: 1, ParentID: 2, Version: 5}}, LocalSessions: map[string]model.LocalSession{}}
	agent := &model.AgentDefinition{ID: "agent:1", Name: "Spellchecker", Version: 6}
	human := model.Account{ID: "u"}
	at := freezeAttribution(st, human, 1, 3, agent, "usage:1", false)
	if at.InitiatorID != "u" || at.CallerKind != "human" || at.ProjectVersion != 4 || at.TicketVersion != 5 || at.EpicID != 2 || at.EpicVersion != 3 || at.GoalID != 1 || at.GoalVersion != 2 || at.AgentVersion != 6 || at.HierarchyStatus != "complete" {
		t.Fatalf("bad snapshot: %+v", at)
	}
	st.LocalSessions["H1"] = model.LocalSession{ID: "H1", Creator: "u", ProjectID: 1, Attribution: at}
	delegated := freezeAttribution(st, model.Account{ID: "agent:H1", ServiceSessionID: "H1"}, 1, 3, nil, "", false)
	if delegated.CallerKind != "agent" || delegated.ParentSessionID != "H1" || delegated.InitiatorID != "u" {
		t.Fatalf("bad delegation: %+v", delegated)
	}
	system := freezeAttribution(st, model.Account{ID: "admin"}, 1, 3, nil, "", true)
	if system.InitiatorID != "" || system.CallerKind != "system" {
		t.Fatal("supervisor owner was misidentified as a requester")
	}
	row := model.LocalSession{ProjectID: 1, Creator: "u", Attribution: at}
	epic := st.Tickets[2]
	epic.ProjectID = 2
	st.Tickets[2] = epic
	if at.EpicID != 2 || at.ProjectID != 1 || at.EpicVersion != 3 {
		t.Fatal("snapshot changed after move")
	}
	key, label := usageGroup(st, human, row, "epic")
	if key != "restricted" || label != "Restricted or moved context" {
		t.Fatalf("moved epic leaked: %q %q", key, label)
	}
	legacy := retainedAttribution(model.LocalSession{ProjectID: 1, Creator: "u", TicketID: 3})
	if legacy.EpicID != 0 || legacy.HierarchyStatus != "unknown" || legacy.InitiatorID != "u" {
		t.Fatalf("legacy inferred unrecorded hierarchy: %+v", legacy)
	}
	legacyManaged := retainedAttribution(model.LocalSession{ProjectID: 1, Creator: "admin", Managed: &model.ManagedLaunch{}})
	if legacyManaged.InitiatorID != "" {
		t.Fatal("legacy supervisor owner was presented as requester")
	}
	privateKey, privateLabel := usageGroup(st, human, model.LocalSession{ProjectID: 1, Creator: "other"}, "user")
	if privateKey != "private" || privateLabel != "Private sessions" {
		t.Fatal("another user's private console identity was disclosed")
	}
}
