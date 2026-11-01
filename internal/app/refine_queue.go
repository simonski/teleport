package app

import "teleport/internal/model"

func refinementQueueAvailable(st *model.State, t model.Ticket) bool {
	if t.Protected || t.Archived || t.Deleted || busy(t) || t.Delivery != nil || t.Refinement != nil && t.Refinement.AcceptedBy != "" || t.Question != nil && !t.Question.Resolved && !t.Question.Obsolete {
		return false
	}
	for _, session := range st.LocalSessions {
		if session.TicketID == t.ID && activeLocal(session) {
			return false
		}
	}
	for _, revision := range st.History[t.ID] {
		prior := revision.Snapshot
		if prior.Agent && (prior.Delivery != nil || contains([]string{"Doing", "Review", "Approval", "Done", "Rebase"}, prior.Status)) {
			return false
		}
	}
	return true
}
