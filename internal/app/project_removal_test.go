package app

import (
	"encoding/json"
	"testing"

	"teleport/internal/model"
)

func removalFixture() *model.State {
	return &model.State{Revision: 7, Projects: map[int64]model.Project{1: {ID: 1, Name: "One"}, 2: {ID: 2, Name: "Two"}}, Tickets: map[int64]model.Ticket{10: {ID: 10, ProjectID: 1}, 20: {ID: 20, ProjectID: 2}}, History: map[int64][]model.Revision{10: {{Version: 1}}}, Ideas: map[string]model.Idea{"i": {ID: "i", ProjectID: 1, Placement: "project"}}, IdeaHistory: map[string][]model.IdeaRevision{"i": {{Sequence: 1}}}, IdeaProposals: map[string]model.IdeaProposal{"p": {ID: "p", IdeaID: "i"}}, IdeaAcceptances: map[string]model.IdeaAcceptance{"p": {IdeaID: "i"}}, IdeaLinks: []model.IdeaLink{{IdeaID: "i", TicketID: 10}}, LocalSessions: map[string]model.LocalSession{"s": {ID: "s", ProjectID: 1, State: "exited"}}, LocalInputs: map[string]model.LocalInput{"s:1": {SessionID: "s"}}, Workflows: map[string]model.Workflow{"local": {ProjectID: 1}, "shared": {}}, Preferences: map[string]model.Preferences{"a": {DefaultProject: 1}}, Receipts: map[string]model.Receipt{"audit": {Fingerprint: "retained"}}}
}
func removalRaw(id int64, revision int64, confirm bool) json.RawMessage {
	b, _ := json.Marshal(ProjectRemovalInput{ID: id, ExpectedRevision: revision, Confirm: confirm})
	return b
}
func TestProjectRemovalAuthorizationConfirmationAndCleanup(t *testing.T) {
	admin := model.Account{ID: "a", Administrator: true}
	st := removalFixture()
	for _, actor := range []model.Account{{ID: "x"}, {ID: "a", Administrator: true, ServiceRunID: "script"}, {ID: "a", Administrator: true, ServiceSessionID: "session"}} {
		if _, _, err := removeProject(st, actor, removalRaw(1, 7, true), false); err == nil {
			t.Fatal("unauthorized removal")
		}
	}
	if _, _, err := removeProject(st, admin, removalRaw(1, 7, false), false); err == nil {
		t.Fatal("missing confirmation accepted")
	}
	if _, _, err := removeProject(st, admin, removalRaw(1, 6, true), false); err == nil {
		t.Fatal("stale confirmation accepted")
	}
	value, changed, err := removeProject(st, admin, removalRaw(1, 0, false), true)
	if err != nil || changed || value.(ProjectRemovalPreview).Tickets != 1 {
		t.Fatal(value, changed, err)
	}
	_, changed, err = removeProject(st, admin, removalRaw(1, 7, true), false)
	if err != nil || !changed {
		t.Fatal(err)
	}
	if len(st.Projects) != 1 || len(st.Tickets) != 1 || len(st.Ideas) != 0 || len(st.History) != 0 || len(st.LocalSessions) != 0 || len(st.LocalInputs) != 0 || len(st.IdeaLinks) != 0 || len(st.IdeaProposals) != 0 || len(st.IdeaAcceptances) != 0 || len(st.Workflows) != 1 || st.Preferences["a"].DefaultProject != 0 || len(st.Receipts) != 1 {
		t.Fatal("unexpected retained or removed data", st)
	}
}
func TestProjectRemovalRejectsActiveSessionsAndForeignReferencesAtomically(t *testing.T) {
	admin := model.Account{Administrator: true}
	for _, mutate := range []func(*model.State){
		func(st *model.State) { t := st.Tickets[10]; t.Active = true; st.Tickets[10] = t }, func(st *model.State) { s := st.LocalSessions["s"]; s.State = "running"; st.LocalSessions["s"] = s }, func(st *model.State) { t := st.Tickets[20]; t.Dependencies = []int64{10}; st.Tickets[20] = t }, func(st *model.State) { st.IdeaLinks = append(st.IdeaLinks, model.IdeaLink{IdeaID: "i", TicketID: 20}) }, func(st *model.State) {
			st.ScriptRuns = map[string]model.ScriptRun{"s": {Projects: []int64{1}, State: "running"}}
		}} {
		st := removalFixture()
		mutate(st)
		before, _ := json.Marshal(st)
		_, changed, err := removeProject(st, admin, removalRaw(1, 7, true), false)
		after, _ := json.Marshal(st)
		if err == nil || changed || string(before) != string(after) {
			t.Fatal("blocked removal changed state", err)
		}
	}
}
