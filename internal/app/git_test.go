package app_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func repoGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@localhost", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@localhost")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func TestExternalDeliveryUsesOwnedWorktreeAndHumanFastForward(t *testing.T) {
	svc, token, db := fixtureWithStore(t)
	p := call(t, svc, token, "createProject", map[string]any{"name": "Delivery", "key": "delivery", "requestId": "p"}).(model.Project)
	// This explicitly configured workflow uses human Approval without an agent Review phase.
	draft := call(t, svc, token, "copyWorkflow", app.CopyWorkflowInput{SourceKey: "agent@1", ProjectID: p.ID, Title: "External work and human approval", RequestID: "workflow"}).(model.Workflow)
	phases := []model.Phase{}
	for _, phase := range draft.Phases {
		if phase.Role != "Review" {
			phases = append(phases, phase)
		}
	}
	draft.Phases = phases
	rules := []model.Rule{}
	for _, rule := range draft.Rules {
		if rule.Source == "Review" {
			continue
		}
		if rule.Target == "Review" {
			rule.Target = "Approval"
		}
		rules = append(rules, rule)
	}
	draft.Rules = rules
	draft = call(t, svc, token, "saveWorkflow", app.SaveWorkflowInput{Definition: draft, ExpectedRevision: draft.DraftRevision}).(model.Workflow)
	p = call(t, svc, token, "activateWorkflowVersion", app.ActivateWorkflowInput{WorkflowKey: draft.ID + "@draft", ExpectedRevision: draft.DraftRevision, ProjectID: p.ID, ExpectedProjectVersion: p.Version}).(model.Project)
	folder := t.TempDir()
	repo := filepath.Join(folder, "repo")
	os.Mkdir(repo, 0700)
	repoGit(t, repo, "init", "-b", "main")
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("Original\n"), 0600)
	repoGit(t, repo, "add", ".")
	repoGit(t, repo, "commit", "-m", "Initial")
	p = call(t, svc, token, "configureLocalRepository", app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version, Repository: repo, StorageRoot: filepath.Join(folder, "worktrees")}).(model.Project)
	ticket := call(t, svc, token, "createTicket", map[string]any{"projectId": p.ID, "title": "Deliver the change", "requestId": "t"}).(model.Ticket)
	settings := call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "demo"}).(model.Settings)
	ticket = call(t, svc, token, "requestRefinement", app.VersionInput{ID: ticket.ID, ExpectedVersion: ticket.Version}).(model.Ticket)
	refined := call(t, svc, token, "runDemoStage", app.DemoStageInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "refine"}).(app.DemoStageResult)
	ticket = call(t, svc, token, "acceptRefinement", app.AcceptScopeInput{ID: ticket.ID, ExpectedVersion: refined.Ticket.Version, ProposalID: refined.Ticket.Refinement.ID}).(model.Ticket)
	call(t, svc, token, "setExecutionMode", app.ExecutionModeInput{Mode: "off", ExpectedVersion: settings.Version})
	p = call(t, svc, token, "configureLocalRepository", app.RepositoryInput{ProjectID: p.ID, ExpectedVersion: p.Version, Repository: repo, StorageRoot: filepath.Join(folder, "new-worktrees")}).(model.Project)
	stale, _ := json.Marshal(app.GitTaskInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "stale"})
	if _, err := svc.Call(context.Background(), token, "prepareTaskWorktree", stale); err == nil {
		t.Fatal("repository change reused stale scope acceptance")
	}
	basis := call(t, svc, token, "getTicketAgreements", app.IDInput{ID: ticket.ID}).(app.AgreementBasis)
	if !basis.Stale {
		t.Fatal("repository change was not exposed as stale readiness")
	}
	ticket = call(t, svc, token, "markReady", app.MarkReadyInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Basis: basis.CurrentBasis, Confirm: true}).(model.Ticket)
	ticket = call(t, svc, token, "prepareTaskWorktree", app.GitTaskInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "prepare"}).(model.Ticket)
	work := ticket.Delivery.Worktree.Path
	os.WriteFile(filepath.Join(work, "README.md"), []byte("Delivered\n"), 0600)
	repoGit(t, work, "add", ".")
	repoGit(t, work, "commit", "-m", "Deliver fixture change")
	ticket = call(t, svc, token, "recordTaskDelivery", app.GitTaskInput{ID: ticket.ID, ExpectedVersion: ticket.Version, RequestID: "delivery"}).(model.Ticket)
	if ticket.Status != "Approval" || ticket.Delivery.Candidate == nil || !ticket.Delivery.Candidate.ChecksPassed {
		t.Fatal("real candidate was not captured for explicit human Approval")
	}
	if err := db.Update(context.Background(), func(st *model.State) (bool, error) {
		current := st.Tickets[ticket.ID]
		current.StopRequested = true
		st.Tickets[current.ID] = current
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	blocked, _ := json.Marshal(app.ApprovalInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Confirm: true})
	if _, err := svc.Call(context.Background(), token, "approveTicket", blocked); err == nil {
		t.Fatal("durable stop intent was ignored")
	}
	before, _ := os.ReadFile(filepath.Join(repo, "README.md"))
	if string(before) != "Original\n" {
		t.Fatal("stopped approval changed main before refusing finalization")
	}
	ticket = call(t, svc, token, "resumeTicket", app.VersionInput{ID: ticket.ID, ExpectedVersion: ticket.Version}).(model.Ticket)
	approval := app.ApprovalInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Confirm: true}
	ticket = call(t, svc, token, "approveTicket", approval).(model.Ticket)
	replayed := call(t, svc, token, "approveTicket", approval).(model.Ticket)
	if replayed.Version != ticket.Version {
		t.Fatal("approval retry created another completion")
	}
	if ticket.Status != "Done" || ticket.AgentReview != nil {
		t.Fatal("approval fabricated agent evidence or failed to complete")
	}
	agreement := ticket.Agreements[len(ticket.Agreements)-1]
	if agreement.Kind != "Done" || agreement.Commit != ticket.Delivery.Candidate.Commit || agreement.RecordVersion != ticket.Version || agreement.Workflow != ticket.Workflow || agreement.ProjectID != ticket.ProjectID {
		t.Fatal("completion did not freeze its exact candidate and record basis")
	}
	contents, _ := os.ReadFile(filepath.Join(repo, "README.md"))
	if string(contents) != "Delivered\n" {
		t.Fatal("Done recorded before real main integration")
	}
}
