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
	"teleport/internal/store"
)

func scriptFixture(t *testing.T) (*app.Service, string, string, *store.Store, model.Project) {
	t.Helper()
	svc, human, db := fixtureWithStore(t)
	project := call(t, svc, human, "createProject", app.CreateProjectInput{Name: "Scripts", RequestID: "scripts"}).(model.Project)
	secret := strings.Repeat("d", 64)
	hash := sha256.Sum256([]byte(secret))
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.ScriptRuns = map[string]model.ScriptRun{"J1": {ID: "J1", Number: 1, Creator: "admin", RequestID: "script", Source: "private source", Mode: "script", State: "running", TokenHash: hex.EncodeToString(hash[:]), Projects: []int64{project.ID}, ExpiresAt: time.Now().Add(time.Hour), Calls: []model.ScriptCall{}}}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	return svc, human, secret, db, project
}
func scriptInvoke(t *testing.T, svc *app.Service, secret, number, operation string, args any) app.ScriptCallResult {
	t.Helper()
	raw, _ := json.Marshal(args)
	return call(t, svc, secret, "scriptCall", app.ScriptCallInput{ID: "J1", CallID: number, Operation: operation, Arguments: raw}).(app.ScriptCallResult)
}
func TestScriptReceiptsNoBorrowedSignInAndPrivateContext(t *testing.T) {
	svc, human, secret, db, project := scriptFixture(t)
	for _, operation := range []string{"getSession", "login", "listProjects", "approveTicket", "confirmScriptCall", "getScriptRun"} {
		if _, err := svc.Call(context.Background(), secret, operation, json.RawMessage(`{}`)); err == nil {
			t.Fatal("script credential escaped binding dispatch", operation)
		}
	}
	input := app.CreateTicketInput{ProjectID: project.ID, Title: "Script ticket", RequestID: "script-ticket"}
	first := scriptInvoke(t, svc, secret, "1", "createTicket", input)
	again := scriptInvoke(t, svc, secret, "1", "createTicket", input)
	if first.State != "completed" || string(first.Result) != string(again.Result) {
		t.Fatal(first, again)
	}
	var ticket model.Ticket
	json.Unmarshal(first.Result, &ticket)
	history := call(t, svc, human, "listTicketHistory", app.IDInput{ID: ticket.ID}).(app.Page[model.Revision])
	last := history.Items[len(history.Items)-1]
	if last.ScriptRunID != "J1" || last.ScriptCallID != "1" || last.Actor != "admin" {
		t.Fatal("missing script audit", last)
	}
	call(t, svc, human, "logout", struct{}{})
	if after := scriptInvoke(t, svc, secret, "2", "listTickets", app.ListInput{ProjectID: project.ID}); after.State != "completed" {
		t.Fatal("logout stopped the bounded job", after)
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		a := st.Accounts["admin"]
		a.Suspended = true
		st.Accounts["admin"] = a
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		a := st.Accounts["admin"]
		a.Suspended = false
		st.Accounts["admin"] = a
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(app.ScriptCallInput{ID: "J1", CallID: "3", Operation: "listProjects", Arguments: json.RawMessage(`{}`)})
	if _, err := svc.Call(context.Background(), secret, "scriptCall", raw); err == nil {
		t.Fatal("suspension/reactivation before the pump resurrected job authority")
	}
}
func TestScriptHumanGateRequiresExactFreshAssent(t *testing.T) {
	for _, decision := range []string{"approve", "decline", "stale"} {
		t.Run(decision, func(t *testing.T) {
			svc, human, secret, _, project := scriptFixture(t)
			ticket := call(t, svc, human, "createTicket", app.CreateTicketInput{ProjectID: project.ID, Title: "Scope", RequestID: "scope"}).(model.Ticket)
			call(t, svc, human, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"})
			ticket = call(t, svc, human, "requestRefinement", app.VersionInput{ID: ticket.ID, ExpectedVersion: ticket.Version}).(model.Ticket)
			ticket = call(t, svc, human, "runDemoStage", app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "scope-refine"}).(app.DemoStageResult).Ticket
			input := app.AcceptScopeInput{ID: ticket.ID, ExpectedVersion: ticket.Version, ProposalID: ticket.Refinement.ID}
			waiting := scriptInvoke(t, svc, secret, "1", "acceptRefinement", input)
			if waiting.State != "waiting" {
				t.Fatal("Run substituted for scope acceptance", waiting)
			}
			view := call(t, svc, human, "getScriptRun", app.ScriptIDInput{ID: "J1"}).(app.ScriptView)
			pending := view.Calls[0]
			confirmation := app.ConfirmScriptInput{ID: "J1", CallID: "1", Fingerprint: pending.Fingerprint, Decision: "approve", Confirm: true}
			if decision == "stale" {
				call(t, svc, human, "pauseTicket", app.VersionInput{ID: ticket.ID, ExpectedVersion: ticket.Version})
				raw, _ := json.Marshal(confirmation)
				if _, err := svc.Call(context.Background(), human, "confirmScriptCall", raw); err == nil {
					t.Fatal("stale assent accepted")
				}
				confirmation.Decision = "decline"
			} else {
				confirmation.Decision = decision
			}
			call(t, svc, human, "confirmScriptCall", confirmation)
			resolved := scriptInvoke(t, svc, secret, "1", "acceptRefinement", input)
			current := call(t, svc, human, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket)
			if decision == "approve" {
				if resolved.State != "completed" || current.Status != "Ready" {
					t.Fatal("qualified exact assent failed", resolved, current.Status)
				}
			} else if resolved.State != "declined" || current.Status != "Refine" {
				t.Fatal("decline changed domain workflow", resolved, current.Status)
			}
		})
	}
}

func TestScriptAdminMetadataAndRevokedContextRemainPrivate(t *testing.T) {
	svc, human, _, db, project := scriptFixture(t)
	otherToken := strings.Repeat("9", 64)
	hash := sha256.Sum256([]byte(otherToken))
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		st.Accounts["other"] = model.Account{ID: "other", Username: "other", Administrator: true}
		st.Sessions[hex.EncodeToString(hash[:])] = model.Session{AccountID: "other", Expires: time.Now().Add(time.Hour)}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	metadata := call(t, svc, otherToken, "listScriptRuns", app.ListInput{}).(app.ScriptPage)
	encoded, _ := json.Marshal(metadata)
	if len(metadata.Items) != 1 || strings.Contains(string(encoded), "private source") || strings.Contains(string(encoded), "requestId") {
		t.Fatal("administrator received private contents", string(encoded))
	}
	raw, _ := json.Marshal(app.ScriptIDInput{ID: "J1"})
	if _, err := svc.Call(context.Background(), otherToken, "getScriptRun", raw); err == nil {
		t.Fatal("administrator read private source")
	}
	if view := call(t, svc, human, "getScriptRun", app.ScriptIDInput{ID: "J1"}).(app.ScriptView); view.Source != "private source" {
		t.Fatal(view)
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		a := st.Accounts["admin"]
		a.Administrator = false
		st.Accounts["admin"] = a
		p := st.Projects[project.ID]
		delete(p.Members, "admin")
		st.Projects[p.ID] = p
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Call(context.Background(), human, "getScriptRun", raw); err == nil {
		t.Fatal("revoked project access retained run contents")
	}
	stopped := call(t, svc, otherToken, "stopScriptRun", app.StopScriptInput{ID: "J1", Confirm: true}).(app.ScriptInfo)
	if !stopped.StopRequested || stopped.StopActor != "other" || stopped.RequestID != "" {
		t.Fatal("administrator stop leaked contents or lost audit", stopped)
	}
}

func TestScriptHumanCompletionUsesSemanticGateAndCanContinueAfterRejection(t *testing.T) {
	svc, human, secret, db, project := scriptFixture(t)
	rejected := scriptInvoke(t, svc, secret, "1", "missingBinding", map[string]any{})
	if rejected.State != "rejected" || rejected.Error.Code != "binding_unavailable" {
		t.Fatal(rejected)
	}
	manual := false
	ticket := call(t, svc, human, "createTicket", app.CreateTicketInput{ProjectID: project.ID, Title: "Human completion", Agent: &manual, RequestID: "human-completion"}).(model.Ticket)
	ticket = call(t, svc, human, "transitionTicket", app.TransitionInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Target: "Doing"}).(model.Ticket)
	// A custom destination ID must retain the semantic Done acceptance gate.
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		w := st.Workflows[ticket.Workflow]
		for i := range w.Phases {
			if w.Phases[i].Role == "Done" {
				old := w.Phases[i].ID
				w.Phases[i].ID = "finish-here"
				for j := range w.Rules {
					if w.Rules[j].Source == old {
						w.Rules[j].Source = "finish-here"
					}
					if w.Rules[j].Target == old {
						w.Rules[j].Target = "finish-here"
					}
				}
			}
		}
		st.Workflows[ticket.Workflow] = w
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	input := app.TransitionInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Target: "finish-here", Confirm: true}
	waiting := scriptInvoke(t, svc, secret, "2", "transitionTicket", input)
	if waiting.State != "waiting" {
		t.Fatal("script self-confirmed semantic Done", waiting)
	}
	view := call(t, svc, human, "getScriptRun", app.ScriptIDInput{ID: "J1"}).(app.ScriptView)
	call(t, svc, human, "confirmScriptCall", app.ConfirmScriptInput{ID: "J1", CallID: "2", Fingerprint: view.Calls[1].Fingerprint, Decision: "approve", Confirm: true})
	completed := scriptInvoke(t, svc, secret, "2", "transitionTicket", input)
	if completed.State != "completed" {
		t.Fatal(completed)
	}
	current := call(t, svc, human, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket)
	if current.Status != "Done" || len(current.Agreements) == 0 {
		t.Fatal("human completion agreement missing", current)
	}
}
