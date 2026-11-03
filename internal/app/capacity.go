package app

import "teleport/internal/model"

func refinePool(p model.Phase) bool {
	return p.Role == "Refine" || p.Role == "Decompose"
}

// Count retained reservations, including uncertain sessions, once per owned process.
func projectAgentCapacity(st *model.State, projectID int64) error {
	maximum := st.Projects[projectID].MaxAgents
	if maximum == nil {
		return nil
	}
	used := 0
	counted := map[string]bool{}
	for _, ticket := range st.Tickets {
		if ticket.ProjectID != projectID || !ticket.Agent || !busy(ticket) {
			continue
		}
		used++
		if ticket.Execution != "" {
			counted[ticket.Execution] = true
		}
	}
	for _, row := range st.LocalSessions {
		if row.ProjectID == projectID && activeLocal(row) && !counted["local:"+row.ID] {
			used++
		}
	}
	if used >= *maximum {
		return failure(409, "project_agent_limit", "The project agent limit is fully reserved")
	}
	return nil
}

func projectAllowsHarness(project model.Project, harness string) bool {
	return project.AllowedHarnesses == nil || contains(*project.AllowedHarnesses, harness)
}

// phaseCapacity checks reservations, not observed process liveness. Unknown and
// stopping ownership keeps its place until the canonical cleanup releases it.
func phaseCapacity(st *model.State, t model.Ticket, phase model.Phase) error {
	if err := projectAgentCapacity(st, t.ProjectID); err != nil {
		return err
	}
	project := st.Projects[t.ProjectID]
	limit := 1
	if project.CapacityConfigured {
		limit = project.DoingWorkers
		if refinePool(phase) {
			limit = project.RefineWorkers
		}
	}
	if limit == 0 {
		return failure(409, "capacity_disabled", "The shared execution pool has zero workers")
	}
	if phase.Workers != nil && *phase.Workers == 0 {
		return failure(409, "lane_capacity_disabled", "The pinned phase has zero workers")
	}
	workflow := st.Workflows[t.Workflow]
	poolUsed, laneUsed := 0, 0
	count := func(other model.Ticket) {
		otherWorkflow := st.Workflows[other.Workflow]
		otherPhase, _ := phaseByID(otherWorkflow, other.Phase)
		if refinePool(phase) == refinePool(otherPhase) {
			poolUsed++
		}
		if workflow.ID == otherWorkflow.ID && phase.ID == other.Phase {
			laneUsed++
		}
	}
	countedSessions := map[string]bool{}
	for _, other := range st.Tickets {
		if other.ProjectID != t.ProjectID || !busy(other) {
			continue
		}
		// A ticket may already show Cancelled while its native implementation
		// still owns the source-phase reservation.
		if other.Execution != "" {
			countedSessions[other.Execution] = true
			if other.Delivery != nil && other.Delivery.SourcePhase != "" {
				other.Phase = other.Delivery.SourcePhase
			}
			for _, row := range st.LocalSessions {
				if other.Execution == "local:"+row.ID && row.Managed != nil {
					other.Workflow, other.Phase = row.Managed.Workflow, row.Managed.Phase
				}
			}
		}
		count(other)
	}
	for _, row := range st.LocalSessions {
		if row.ProjectID != t.ProjectID || row.TicketID == 0 || !activeLocal(row) || countedSessions["local:"+row.ID] {
			continue
		}
		other := st.Tickets[row.TicketID]
		if other.Delivery != nil && other.Delivery.SourcePhase != "" {
			other.Phase = other.Delivery.SourcePhase
		}
		if row.Managed != nil {
			other.Workflow, other.Phase = row.Managed.Workflow, row.Managed.Phase
		}
		count(other)
	}
	if poolUsed >= limit {
		return failure(409, "capacity_full", "The shared project pool is fully reserved")
	}
	if phase.Workers != nil && laneUsed >= *phase.Workers {
		return failure(409, "lane_capacity_full", "The pinned phase quota is fully reserved")
	}
	return nil
}
