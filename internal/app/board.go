package app

import (
	"context"
	"encoding/json"
	"sort"
	"teleport/internal/localruntime"
	"teleport/internal/model"
	"time"
)

type BoardExecutionInput struct {
	ProjectID   int64  `json:"projectId"`
	WorkflowKey string `json:"workflowKey"`
}
type BoardPhaseExecution struct {
	PhaseID         string  `json:"phaseId"`
	Active          int     `json:"active"`
	Capacity        int     `json:"capacity"`
	Reserved        int     `json:"reserved"`
	Unresolved      int     `json:"unresolved"`
	ActiveTicketIDs []int64 `json:"activeTicketIds"`
}
type BoardExecution struct {
	ProjectID      int64                 `json:"projectId"`
	ProjectVersion int64                 `json:"projectVersion"`
	WorkflowKey    string                `json:"workflowKey"`
	Braked         bool                  `json:"braked"`
	ProjectBraked  bool                  `json:"projectBraked"`
	SystemBraked   bool                  `json:"systemBraked"`
	Phases         []BoardPhaseExecution `json:"phases"`
}

func (s *Service) boardExecution(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in BoardExecutionInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	s.localMu.Lock()
	defer s.localMu.Unlock()
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := directoryActor(st, token)
	if err != nil {
		return nil, err
	}
	if err = require(st, a, in.ProjectID, "Viewer"); err != nil {
		return nil, err
	}
	initialWorkflow, err := workflowRead(st, a, in.WorkflowKey)
	if err != nil {
		return nil, err
	}
	if !initialWorkflow.Published || initialWorkflow.ProjectID != 0 && initialWorkflow.ProjectID != in.ProjectID {
		return nil, failure(422, "invalid_board_workflow", "Select a published workflow for this project")
	}
	observed := map[string]localruntime.Status{}
	working := map[string]bool{}
	turnUnknown := map[string]bool{}
	if s.localClient != nil {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		needsObservation := false
		for _, row := range st.LocalSessions {
			if row.ProjectID == in.ProjectID && row.TicketID != 0 && activeLocal(row) {
				needsObservation = true
				break
			}
		}
		if needsObservation {
			if out, callErr := s.localClient.Call(probe, localruntime.Request{Operation: "list"}); callErr == nil {
				for _, row := range out.Items {
					observed[row.ID] = row
				}
			}
		}
		for _, row := range st.LocalSessions {
			if row.ProjectID != in.ProjectID || row.TicketID == 0 || !activeLocal(row) {
				continue
			}
			status, ok := observed[row.ID]
			if !ok || status.LaunchID != row.LaunchID || status.Generation != row.Generation || status.State != "running" || row.StopRequested {
				continue
			}
			if row.Harness != "codex" {
				working[row.ID] = true
				continue
			}
			out, callErr := s.localClient.Call(probe, daemonRequest(row, "nativeTurn"))
			if callErr != nil {
				turnUnknown[row.ID] = true
			} else if out.Turn != nil {
				working[row.ID] = out.Turn.State == "inProgress"
				turnUnknown[row.ID] = out.Turn.State == "unknown"
			}
		}
	}
	st, err = s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err = directoryActor(st, token)
	if err != nil {
		return nil, err
	}
	if err = require(st, a, in.ProjectID, "Viewer"); err != nil {
		return nil, err
	}
	w, err := workflowRead(st, a, in.WorkflowKey)
	if err != nil {
		return nil, err
	}
	p := st.Projects[in.ProjectID]
	result := BoardExecution{ProjectID: p.ID, ProjectVersion: p.Version, WorkflowKey: in.WorkflowKey, Braked: p.LaneLocks[w.ID+"#*"], ProjectBraked: p.Handbrake, SystemBraked: st.Settings.Handbrake, Phases: []BoardPhaseExecution{}}
	reserved := map[int64]model.Ticket{}
	sessions := map[int64]model.LocalSession{}
	for _, t := range st.Tickets {
		if t.ProjectID == p.ID && busy(t) {
			reserved[t.ID] = t
		}
	}
	for _, row := range st.LocalSessions {
		if row.ProjectID == p.ID && row.TicketID != 0 && activeLocal(row) {
			reserved[row.TicketID] = st.Tickets[row.TicketID]
			sessions[row.TicketID] = row
		}
	}
	for _, phase := range w.Phases {
		row := BoardPhaseExecution{PhaseID: phase.ID, ActiveTicketIDs: []int64{}}
		if phase.Agentic {
			row.Capacity = 1
			if p.CapacityConfigured {
				row.Capacity = p.DoingWorkers
				if refinePool(phase) {
					row.Capacity = p.RefineWorkers
				}
			}
			if phase.Workers != nil && *phase.Workers < row.Capacity {
				row.Capacity = *phase.Workers
			}
		}
		for id, t := range reserved {
			other := st.Workflows[t.Workflow]
			source := t.Phase
			if t.Delivery != nil && t.Delivery.SourcePhase != "" {
				source = t.Delivery.SourcePhase
			}
			if other.ID != w.ID || source != phase.ID {
				continue
			}
			row.Reserved++
			session, ok := sessions[id]
			status, seen := observed[session.ID]
			if !ok || !seen || status.LaunchID != session.LaunchID || status.Generation != session.Generation || status.State == "unknown" || turnUnknown[session.ID] {
				row.Unresolved++
				continue
			}
			if session.State == "running" && status.State == "running" && !session.StopRequested && session.Attention == "" && status.Attention == "" && working[session.ID] {
				row.Active++
				if !t.Archived && !t.Deleted {
					row.ActiveTicketIDs = append(row.ActiveTicketIDs, id)
				}
			}
		}
		sort.Slice(row.ActiveTicketIDs, func(i, j int) bool { return row.ActiveTicketIDs[i] < row.ActiveTicketIDs[j] })
		result.Phases = append(result.Phases, row)
	}
	return result, nil
}
