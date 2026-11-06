package app

import "teleport/internal/model"

// ensurePersonalProject is called in the successful sign-in transaction.
func ensurePersonalProject(st *model.State, a model.Account) {
	for _, p := range st.Projects {
		if p.PersonalOwner == a.ID {
			return
		}
	}
	var key string
	for {
		key = "personal-" + randomID()[:24]
		used := false
		for _, p := range st.Projects {
			if p.Key == key {
				used = true
				break
			}
		}
		if !used {
			break
		}
	}
	st.NextProjectID++
	p := model.Project{ID: st.NextProjectID, Version: 1, Key: key, Name: "Personal", PersonalOwner: a.ID, Creator: a.ID, Root: defaultProjectRoot, Members: map[string]string{a.ID: "Maintainer"}, HumanWorkflow: "human@1", AgentWorkflow: "agent@1"}
	goal := newTicket(st, p, a, "goal", "Unassigned", false, true)
	epic := newTicket(st, p, a, "epic", "Unassigned", false, true)
	epic.ParentID = goal.ID
	p.UnassignedGoal, p.UnassignedEpic = goal.ID, epic.ID
	st.Projects[p.ID] = p
	saveTicket(st, &goal, a, "createPersonalProject")
	saveTicket(st, &epic, a, "createPersonalProject")
}
