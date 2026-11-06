package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"teleport/internal/model"
)

type ProjectRemovalInput struct {
	ID               int64 `json:"id"`
	ExpectedRevision int64 `json:"expectedRevision,omitempty"`
	Confirm          bool  `json:"confirm,omitempty"`
}
type ProjectRemovalPreview struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Revision  int64  `json:"revision"`
	Tickets   int    `json:"tickets"`
	Ideas     int    `json:"ideas"`
	Sessions  int    `json:"sessions"`
	Workflows int    `json:"workflows"`
	Warning   string `json:"warning"`
}

func removalPreview(st *model.State, a model.Account, id int64) (ProjectRemovalPreview, error) {
	if !a.Administrator || a.ServiceSessionID != "" || a.ServiceRunID != "" {
		return ProjectRemovalPreview{}, failure(403, "forbidden", "Only a human installation administrator can remove projects")
	}
	p, ok := st.Projects[id]
	if !ok {
		return ProjectRemovalPreview{}, failure(404, "not_found", "Project not found")
	}
	if p.PersonalOwner != "" {
		return ProjectRemovalPreview{}, failure(404, "not_found", "Project not found")
	}
	r := ProjectRemovalPreview{ID: id, Name: p.Name, Revision: st.Revision, Warning: "Project tickets, Ideas, their history, project workflows and session records are removed. Retained script records, request receipts, backups and files on disk are not erased. Active work, sessions, scripts and cross-project references must be resolved first."}
	for _, t := range st.Tickets {
		if t.ProjectID == id {
			r.Tickets++
		}
	}
	for _, i := range st.Ideas {
		if i.ProjectID == id && i.Placement == "project" {
			r.Ideas++
		}
	}
	for _, s := range st.LocalSessions {
		if s.ProjectID == id {
			r.Sessions++
		}
	}
	for _, w := range st.Workflows {
		if w.ProjectID == id {
			r.Workflows++
		}
	}
	return r, nil
}
func removeProject(st *model.State, a model.Account, raw json.RawMessage, preview bool) (any, bool, error) {
	var in ProjectRemovalInput
	if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	impact, err := removalPreview(st, a, in.ID)
	if err != nil {
		return nil, false, err
	}
	if preview {
		return impact, false, nil
	}
	if !in.Confirm {
		return nil, false, failure(422, "confirmation_required", "Confirm the displayed project removal impact")
	}
	if in.ExpectedRevision != st.Revision {
		return nil, false, failure(409, "version_conflict", "Installation changed; review project removal again")
	}
	ticketIDs := map[int64]bool{}
	ideaIDs := map[string]bool{}
	for id, t := range st.Tickets {
		if t.ProjectID == in.ID {
			if busy(t) {
				return nil, false, failure(409, "active_ticket", "Stop and reconcile all project work and delivery operations before removal")
			}
			ticketIDs[id] = true
		}
	}
	for id, i := range st.Ideas {
		if i.ProjectID == in.ID && i.Placement == "project" {
			ideaIDs[id] = true
		}
	}
	for _, s := range st.LocalSessions {
		if s.ProjectID == in.ID && (activeLocal(s) || s.Managed != nil && !s.Managed.Applied) {
			return nil, false, failure(409, "active_session", "Stop and reconcile all project sessions before removal")
		}
	}
	for _, run := range st.ScriptRuns {
		for _, id := range run.Projects {
			if id == in.ID && run.State != "completed" && run.State != "failed" && run.State != "stopped" && run.State != "interrupted" {
				return nil, false, failure(409, "active_script", "Stop and reconcile project scripts before removal")
			}
		}
	}
	for _, run := range st.ScriptRuns {
		for _, projectID := range run.Projects {
			if projectID == in.ID {
				for _, call := range run.Calls {
					if call.State == "dispatching" || call.State == "unknown" {
						return nil, false, failure(409, "unresolved_script", "Reconcile unresolved project script calls before removal")
					}
				}
			}
		}
	}
	for _, t := range st.Tickets {
		if t.ProjectID == in.ID {
			continue
		}
		if ticketIDs[t.ParentID] {
			return nil, false, failure(409, "cross_project_reference", "Resolve cross-project ticket references before removal")
		}
		for _, d := range t.Dependencies {
			if ticketIDs[d] {
				return nil, false, failure(409, "cross_project_reference", "Resolve cross-project ticket dependencies before removal")
			}
		}
	}
	removedWorkflows := map[string]bool{}
	for key, workflow := range st.Workflows {
		if workflow.ProjectID == in.ID {
			removedWorkflows[key] = true
		}
	}
	for _, project := range st.Projects {
		if project.ID != in.ID && (removedWorkflows[project.HumanWorkflow] || removedWorkflows[project.AgentWorkflow]) {
			return nil, false, failure(409, "cross_project_reference", "Resolve shared project workflow references before removal")
		}
	}
	for _, ticket := range st.Tickets {
		if ticket.ProjectID != in.ID && removedWorkflows[ticket.Workflow] {
			return nil, false, failure(409, "cross_project_reference", "Resolve shared ticket workflow references before removal")
		}
	}
	for _, proposal := range st.IdeaProposals {
		if !ideaIDs[proposal.IdeaID] {
			for _, target := range proposal.Targets {
				if ticketIDs[target.ExistingID] || target.Create != nil && target.Create.ProjectID == in.ID {
					return nil, false, failure(409, "cross_project_reference", "Resolve cross-project Idea proposals before removal")
				}
			}
		}
	}
	for _, l := range st.IdeaLinks {
		if ideaIDs[l.IdeaID] != ticketIDs[l.TicketID] && (ideaIDs[l.IdeaID] || ticketIDs[l.TicketID]) {
			return nil, false, failure(409, "cross_project_reference", "Resolve cross-project Idea links before removal")
		}
	}
	for id := range ticketIDs {
		delete(st.Tickets, id)
		delete(st.History, id)
		delete(st.GoalReviews, id)
	}
	for id, proposal := range st.TicketProposals {
		if proposal.ProjectID == in.ID {
			delete(st.TicketProposals, id)
		}
	}
	for id := range ideaIDs {
		delete(st.Ideas, id)
		delete(st.IdeaHistory, id)
	}
	for id, p := range st.IdeaProposals {
		if ideaIDs[p.IdeaID] {
			delete(st.IdeaProposals, id)
		}
	}
	for id, v := range st.IdeaAcceptances {
		if ideaIDs[v.IdeaID] {
			delete(st.IdeaAcceptances, id)
		}
	}
	links := st.IdeaLinks[:0]
	for _, l := range st.IdeaLinks {
		if !ideaIDs[l.IdeaID] && !ticketIDs[l.TicketID] {
			links = append(links, l)
		}
	}
	st.IdeaLinks = links
	sessions := map[string]bool{}
	for id, s := range st.LocalSessions {
		if s.ProjectID == in.ID {
			sessions[id] = true
			delete(st.LocalSessions, id)
		}
	}
	for id, item := range st.LocalInputs {
		if sessions[item.SessionID] {
			delete(st.LocalInputs, id)
		}
	}
	for id, w := range st.Workflows {
		if w.ProjectID == in.ID {
			delete(st.Workflows, id)
		}
	}
	delete(st.GraphLayouts, fmt.Sprintf("project:%d", in.ID))
	delete(st.GraphLayouts, fmt.Sprintf("project:%d:2d", in.ID))
	for key := range st.BacklogOrders {
		if strings.HasSuffix(key, fmt.Sprintf(":%d", in.ID)) {
			delete(st.BacklogOrders, key)
		}
	}
	for key, p := range st.Preferences {
		changed := false
		if p.DefaultProject == in.ID {
			p.DefaultProject = 0
			changed = true
		}
		recent := p.Recent[:0]
		for _, v := range p.Recent {
			if v.ProjectID != in.ID {
				recent = append(recent, v)
			} else {
				changed = true
			}
		}
		p.Recent = recent
		if changed {
			p.Version++
		}
		st.Preferences[key] = p
	}
	for key, l := range st.LastLocations {
		if l.ProjectID == in.ID {
			delete(st.LastLocations, key)
		}
	}
	for key, w := range st.WelcomeSetups {
		if w.ProjectID == in.ID {
			w.ProjectID = 0
			w.ProjectName = ""
			w.ProjectKey = ""
			w.ProjectRoot = ""
			w.Version++
			st.WelcomeSetups[key] = w
		}
	}
	delete(st.Projects, in.ID)
	return impact, true, nil
}
