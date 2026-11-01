package app

import (
	"encoding/json"
	"fmt"
	"time"

	"teleport/internal/model"
)

const petStoreSeed = "petstore-v1"

// Bootstrap only. Subsequent demo resets require their own API/provenance contract.
func seedPetStore(st *model.State) (bool, error) {
	for _, p := range st.Projects {
		if p.DemoSeed == petStoreSeed && p.Simulation {
			return false, nil
		}
		if p.Key == "demo" || sameProjectName(p.Name, "PetStore") {
			return false, failure(409, "demo_collision", "PetStore/demo belongs to a non-demo project; existing work was preserved")
		}
	}
	collections(st)
	ideaCollections(st)
	owner, ok := st.Accounts["admin"]
	if !ok || !owner.Administrator || owner.Suspended {
		return false, failure(409, "demo_owner", "The active bootstrap administrator is required to own the sample project")
	}
	raw, _ := json.Marshal(CreateProjectInput{Name: "PetStore", Key: "demo", Description: "Simulation · a sample online pet store: catalog, cart, checkout, inventory and delivery. No real agents or payments.", RequestID: "init-" + petStoreSeed})
	result, _, err := projectOperations(st, owner, "createProject", raw)
	if err != nil {
		return false, err
	}
	p := result.(model.Project)
	p.Simulation, p.DemoSeed = true, petStoreSeed
	st.Projects[p.ID] = p
	// Refresh the creation receipt with the immutable simulation marker.
	key, fingerprint, _, _ := receipt(st, owner, "createProject", "init-"+petStoreSeed, CreateProjectInput{Name: "PetStore", Key: "demo", Description: p.Description, RequestID: "init-" + petStoreSeed})
	remember(st, key, fingerprint, p)
	now := time.Now().UTC()
	makeTicket := func(kind, title, description string, parent int64, agent bool, phase string) model.Ticket {
		t := newTicket(st, p, owner, kind, title, agent, false)
		t.ParentID, t.Description, t.Criteria = parent, description, "Record the sample outcome and verify the described PetStore behavior."
		t.Labels = []string{"petstore", "simulation"}
		t.CreatedAt = now.Add(-48 * time.Hour)
		t.PhaseEnteredAt = now
		if phase != "Todo" {
			_ = enterPhase(&t, st.Workflows[t.Workflow], phase)
		}
		if phase == "Doing" {
			t.Progress = 50
		}
		if phase == "Done" {
			t.Progress = 100
		}
		if agent && phase != "Todo" && phase != "Refine" {
			basis := executionBasis(st, t)
			t.Refinement = &model.Refinement{ID: randomID(), Text: "Synthetic accepted sample scope: " + description, Criteria: t.Criteria, Basis: basis, AcceptedBasis: basis, AcceptedBy: "demo:seed", AcceptedAt: now, Simulated: true}
		}
		if agent && (phase == "Review" || phase == "Approval") {
			t.Attempts = 1
			t.StageEvents = []model.StageEvent{{AttemptID: randomID(), Phase: "Doing", Outcome: "success", Actor: "demo:seed", At: now, Simulated: true}}
		}
		if phase == "Approval" {
			t.AgentReview = &model.AgentReview{ID: randomID(), Outcome: "success", Findings: "Simulated review only; no code, payment or provider was executed.", At: now, Simulated: true}
			t.Attention = "Simulation: real delivery evidence is unavailable"
		}
		saveTicket(st, &t, owner, "seedPetStore")
		return t
	}
	catalog := makeTicket("goal", "Help customers discover a suitable pet", "Browse an accessible catalog of fish, dogs, cats, birds and reptiles.", 0, false, "Todo")
	checkout := makeTicket("goal", "Make checkout predictable and safe", "Build a reliable cart, simulated payments and clear order confirmation.", 0, false, "Todo")
	operations := makeTicket("goal", "Keep the store ready to fulfil orders", "Track inventory and plan delivery with care information.", 0, false, "Todo")
	epics := []model.Ticket{
		makeTicket("epic", "Species catalog", "Product pages, pictures and care details.", catalog.ID, false, "Todo"),
		makeTicket("epic", "Search and accessibility", "Find pets and navigate with the keyboard.", catalog.ID, false, "Todo"),
		makeTicket("epic", "Shopping cart", "Add, remove and adjust pet supplies.", checkout.ID, false, "Todo"),
		makeTicket("epic", "Checkout and order confirmation", "Use a simulated payment provider and show the order summary.", checkout.ID, false, "Todo"),
		makeTicket("epic", "Inventory management", "Reserve stock and flag low inventory.", operations.ID, false, "Todo"),
		makeTicket("epic", "Delivery and customer care", "Choose safe packaging and communicate delivery status.", operations.ID, false, "Todo"),
	}
	for _, sample := range []struct {
		epic         int
		title, phase string
		agent        bool
	}{
		{0, "Write goldfish care guidance", "Todo", false}, {0, "Select beagle product photographs", "Doing", false}, {0, "Check catalog category names", "Done", false},
		{1, "Add species filters", "Ready", true}, {1, "Improve keyboard navigation", "Refine", true}, {1, "Review search result wording", "Done", false},
		{2, "Persist cart quantities", "Doing", true}, {2, "Explain out-of-stock cart items", "Todo", true}, {2, "Agree whether supplies can ship separately", "Refine", true},
		{3, "Integrate the simulated payment gateway", "Review", true}, {3, "Render order confirmation", "Approval", true}, {3, "Check checkout copy", "Todo", false},
		{4, "Prevent overselling the last aquarium", "Ready", true}, {4, "Add low-stock alerts", "Todo", true}, {4, "Agree inventory import columns", "Done", false},
		{5, "Choose insulated transport packaging", "Todo", false}, {5, "Draft dispatch notification", "Doing", false}, {5, "Show delivery estimates", "Todo", true},
	} {
		t := makeTicket("task", sample.title, sample.title+" for the PetStore demonstration. All execution and outcomes are synthetic.", epics[sample.epic].ID, sample.agent, sample.phase)
		t.Due = now.AddDate(0, 0, 7+sample.epic).Format("2006-01-02")
		t.Start = now.Format("2006-01-02")
		t.ETA = now.AddDate(0, 0, 3+sample.epic).Format("2006-01-02")
		if sample.title == "Agree whether supplies can ship separately" {
			t.Attempts = 1
			t.Question = &model.Question{ID: randomID(), Text: "Can supplies ship separately from live pets?", OriginPhase: t.Phase, OriginAttempt: randomID(), Workflow: t.Workflow, Basis: executionBasis(st, t), AskedAt: now, OriginWaitingSince: now}
			t.Attention = "Question requires an answer"
		}
		saveTicket(st, &t, owner, "seedPetStoreSchedule")
	}
	for _, sample := range []struct{ title, text string }{
		{"Pet matching questionnaire", "Help a new customer choose a pet based on available space and care experience."},
		{"Care reminders", "Offer optional reminders for feeding, cleaning and routine care."},
		{"Adoption partnerships", "Explore a separate adoption directory with local shelters."},
		{"Aquarium starter bundles", "Suggest a compatible tank, filter and supplies for first-time fish owners."},
	} {
		st.NextIdeaID++
		i := model.Idea{ID: fmt.Sprintf("idea:%d", st.NextIdeaID), Placement: "project", ProjectID: p.ID, Creator: owner.ID, Title: sample.title, Text: "PetStore simulation: " + sample.text}
		saveIdea(st, &i, owner, "seedPetStore")
	}
	for _, epic := range epics {
		reconcile(st, epic.ID, owner)
	}
	if st.Preferences == nil {
		st.Preferences = map[string]model.Preferences{}
	}
	if _, exists := st.Preferences[owner.ID]; !exists {
		preferences := personalPreferences(st, owner)
		preferences.Version, preferences.DefaultView, preferences.DefaultProject = 1, "Board", p.ID
		st.Preferences[owner.ID] = preferences
	}
	if st.WelcomeSetups == nil {
		st.WelcomeSetups = map[string]model.WelcomeSetup{}
	}
	if _, exists := st.WelcomeSetups[owner.ID]; !exists {
		st.WelcomeSetups[owner.ID] = model.WelcomeSetup{ID: randomID(), Version: 1, Completed: true, Step: 5, ProjectID: p.ID, ProjectName: p.Name, ProjectKey: p.Key}
	}
	return true, nil
}
