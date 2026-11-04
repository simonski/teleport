package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"teleport/internal/model"
	"teleport/internal/teststore"
)

func managedFixture() (*model.State, model.Account, model.Ticket, model.Phase) {
	a := model.Account{ID: "admin", Administrator: true}
	w := model.SeedWorkflows()["agent@1"]
	ticket := model.Ticket{ID: 3, ProjectID: 1, Kind: "task", Agent: true, Workflow: "agent@1", Phase: "Refine", Status: "Refine", StageVisit: 1, Title: "Refine request"}
	p, _ := phaseByID(w, "Refine")
	st := &model.State{Settings: model.Settings{ExecutionMode: "local"}, Accounts: map[string]model.Account{"admin": a}, Projects: map[int64]model.Project{1: {ID: 1, Repository: &model.RepositoryBinding{Version: 1}, LaneLocks: map[string]bool{}}}, Workflows: map[string]model.Workflow{"agent@1": w}, Tickets: map[int64]model.Ticket{3: ticket}, LocalSessions: map[string]model.LocalSession{}, History: map[int64][]model.Revision{}}
	st.Schema = model.SchemaVersion
	return st, a, ticket, p
}

func TestManagedAdmissionPreservesSafetyGates(t *testing.T) {
	for _, scenario := range []string{"eligible", "off", "system brake", "board brake", "lane brake", "zero pool", "zero lane", "attempt limit", "held", "question", "unknown owner", "simulation"} {
		t.Run(scenario, func(t *testing.T) {
			st, a, ticket, phase := managedFixture()
			project := st.Projects[1]
			switch scenario {
			case "off":
				st.Settings.ExecutionMode = "off"
			case "system brake":
				st.Settings.Handbrake = true
			case "board brake":
				project.LaneLocks["agent#*"] = true
			case "lane brake":
				project.LaneLocks["agent#Refine"] = true
			case "zero pool":
				project.CapacityConfigured = true
				project.RefineWorkers = 0
			case "zero lane":
				zero := 0
				phase.Workers = &zero
			case "attempt limit":
				ticket.Attempts = 20
			case "held":
				ticket.ResumeHold = true
			case "question":
				ticket.Question = &model.Question{Text: "Why?"}
			case "unknown owner":
				st.LocalSessions["H1"] = model.LocalSession{ID: "H1", ProjectID: 1, TicketID: ticket.ID, State: "unknown", Managed: &model.ManagedLaunch{Workflow: ticket.Workflow, Phase: ticket.Phase}}
			case "simulation":
				project.Simulation = true
			}
			st.Projects[1] = project
			if err := managedAdmission(st, a, ticket, phase); (err == nil) != (scenario == "eligible") {
				t.Fatalf("admission %s: %v", scenario, err)
			}
		})
	}
}

func TestManagedExitCannotApplyChangedAuthorityOrInventApproval(t *testing.T) {
	for _, scenario := range []string{"proposal", "malformed", "cancelled", "changed basis", "withdrawn binding", "question"} {
		t.Run(scenario, func(t *testing.T) {
			st, a, ticket, _ := managedFixture()
			m := &model.ManagedLaunch{Workflow: ticket.Workflow, Phase: ticket.Phase, Role: "Refine", Visit: ticket.StageVisit, Basis: executionBasis(st, ticket), Attempt: "attempt", Result: &model.ManagedResult{Outcome: "success", Proposal: "A bounded proposal", Criteria: "A concrete check"}}
			row := model.LocalSession{ID: "H1", Creator: a.ID, ProjectID: 1, TicketID: ticket.ID, State: "exited", BindingVersion: 1, Managed: m}
			switch scenario {
			case "malformed":
				m.Result = nil
			case "cancelled":
				ticket.Status = "Cancelled"
				ticket.Phase = "Cancelled"
			case "changed basis":
				ticket.Title = "Changed"
			case "withdrawn binding":
				p := st.Projects[1]
				p.Repository.Version = 2
				st.Projects[1] = p
			case "question":
				m.Result = &model.ManagedResult{Outcome: "question", Question: "Which behaviour?"}
			}
			st.Tickets[ticket.ID], st.LocalSessions[row.ID] = ticket, row
			db := teststore.Open(t)
			if err := db.Update(context.Background(), func(current *model.State) (bool, error) { *current = *st; return true, nil }); err != nil {
				t.Fatal(err)
			}
			s := New(db)
			ctx := context.WithValue(context.Background(), managedOwnerKey{}, a.ID)
			if err := s.reconcileManaged(ctx, row); err != nil {
				t.Fatal(err)
			}
			after, err := db.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			actual := after.Tickets[ticket.ID]
			if actual.Status == "Done" || actual.Status == "Ready" || actual.Refinement != nil && actual.Refinement.AcceptedBy != "" {
				t.Fatal("agent supplied human acceptance")
			}
			if scenario == "proposal" {
				if actual.Refinement == nil || actual.Attention != "Scope acceptance required" {
					t.Fatalf("proposal not retained: %+v", actual)
				}
			} else if scenario == "question" {
				if actual.Question == nil || actual.Question.OriginAttempt != "attempt" {
					t.Fatal("question lost originating context")
				}
			} else if actual.Refinement != nil {
				t.Fatal("stale or missing result supplied a proposal")
			}
			version := actual.Version
			if err := s.reconcileManaged(ctx, row); err != nil {
				t.Fatal(err)
			}
			after, _ = db.Read(ctx)
			if after.Tickets[ticket.ID].Version != version {
				t.Fatal("outcome replay duplicated a mutation")
			}
		})
	}
}

func TestPromptSnapshotIsImmutableAndPrivate(t *testing.T) {
	st, a, ticket, phase := managedFixture()
	m := &model.ManagedLaunch{Prompt: composeManagedPrompt(st, ticket, phase), Workflow: ticket.Workflow, Phase: ticket.Phase, Basis: executionBasis(st, ticket)}
	st.LocalSessions["H1"] = model.LocalSession{ID: "H1", Creator: a.ID, TicketID: ticket.ID, ProjectID: 1, State: "exited", Managed: m}
	input, _ := json.Marshal(PromptInput{ID: ticket.ID, SessionID: "H1"})
	saved, _, err := ticketPrompt(st, a, input)
	if err != nil {
		t.Fatal(err)
	}
	project := st.Projects[1]
	project.Description = "New project guidance"
	st.Projects[1] = project
	again, _, err := ticketPrompt(st, a, input)
	if err != nil {
		t.Fatal(err)
	}
	if saved.(PromptView).Prompt.Text != again.(PromptView).Prompt.Text {
		t.Fatal("saved prompt was recomputed")
	}
	previewInput, _ := json.Marshal(PromptInput{ID: ticket.ID})
	preview, _, err := ticketPrompt(st, a, previewInput)
	if err != nil {
		t.Fatal(err)
	}
	if preview.(PromptView).Prompt.Text == saved.(PromptView).Prompt.Text {
		t.Fatal("current composition ignored changed context")
	}
	other := model.Account{ID: "other", Administrator: true}
	if _, _, err := ticketPrompt(st, other, input); err == nil {
		t.Fatal("saved launch context leaked to another administrator")
	}
}

func TestHumanTicketPromptPreviewsRefinementWithoutChangingTicket(t *testing.T) {
	st, actor, ticket, _ := managedFixture()
	project := st.Projects[1]
	project.Name, project.Description, project.AgentWorkflow = "Example project", "Project context", "agent@1"
	st.Projects[1] = project
	ticket.Agent, ticket.Workflow, ticket.Phase, ticket.Status = false, "human@1", "Todo", "Todo"
	ticket.Title = "Human request"
	st.Tickets[ticket.ID] = ticket
	input, _ := json.Marshal(PromptInput{ID: ticket.ID})
	result, changed, err := ticketPrompt(st, actor, input)
	if err != nil || changed {
		t.Fatalf("preview failed or mutated state: %v, %v", changed, err)
	}
	view := result.(PromptView)
	if !view.Hypothetical || view.Saved || view.Workflow != "agent@1" || view.Phase != "Refine" {
		t.Fatalf("incorrect preview identity: %+v", view)
	}
	for _, part := range []string{"[system]", "[project]", "Project context", "[task]", "Human request", "[phase]"} {
		if !strings.Contains(view.Prompt.Text, part) {
			t.Errorf("prompt missing %q", part)
		}
	}
	if st.Tickets[ticket.ID].Agent || st.Tickets[ticket.ID].Workflow != "human@1" || len(st.LocalSessions) != 0 {
		t.Fatal("preview changed the ticket or started a session")
	}
}
