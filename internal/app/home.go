package app

import (
	"context"
	"encoding/json"
	"sort"
	"time"
)

type HomeInput struct {
	ProjectID int64 `json:"projectId,omitempty"`
	Days      int   `json:"days,omitempty"`
}
type HomePhase struct {
	Workflow string `json:"workflow"`
	Phase    string `json:"phase"`
	Title    string `json:"title"`
	Count    int    `json:"count"`
}
type HomeSummary struct {
	ProjectID             int64       `json:"projectId"`
	Days                  int         `json:"days"`
	AsOf                  time.Time   `json:"asOf"`
	Total                 int         `json:"total"`
	Completed             int         `json:"completed"`
	InProgress            int         `json:"inProgress"`
	Remaining             int         `json:"remaining"`
	CompletedInPeriod     int         `json:"completedInPeriod"`
	TicketsPerDay         float64     `json:"ticketsPerDay"`
	TicketsPerHour        float64     `json:"ticketsPerHour"`
	ProgressSeconds       float64     `json:"progressSeconds"`
	AttentionWaitSeconds  float64     `json:"attentionWaitSeconds"`
	HistoryStatus         string      `json:"historyStatus"`
	Tokens                *int64      `json:"tokens"`
	AttentionSpentSeconds *float64    `json:"attentionSpentSeconds"`
	Recent                []int64     `json:"recent"`
	Phases                []HomePhase `json:"phases"`
}

func (s *Service) home(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in HomeInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	if in.Days == 0 {
		in.Days = 7
	}
	if in.Days != 7 && in.Days != 30 && in.Days != 90 {
		return nil, failure(422, "invalid_period", "Select 7, 30 or 90 days")
	}
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := directoryActor(st, token)
	if err != nil {
		return nil, err
	}
	if in.ProjectID != 0 {
		if err = require(st, a, in.ProjectID, "Viewer"); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	since := now.Add(-time.Duration(in.Days) * 24 * time.Hour)
	out := HomeSummary{ProjectID: in.ProjectID, Days: in.Days, AsOf: now, HistoryStatus: "retained-events", Recent: []int64{}, Phases: []HomePhase{}}
	phases := map[string]HomePhase{}
	done := map[int64]time.Time{}
	for _, ticket := range st.Tickets {
		if ticket.Protected || ticket.Archived || ticket.Deleted || ticket.Kind != "task" || role(st, a, ticket.ProjectID) == "" || in.ProjectID != 0 && ticket.ProjectID != in.ProjectID {
			continue
		}
		if ticket.Status != "Cancelled" {
			out.Total++
			if ticket.Status == "Done" {
				out.Completed++
			} else {
				out.Remaining++
				if ticket.Status != "Todo" && ticket.Status != "Ready" {
					out.InProgress++
				}
				key := ticket.Workflow + "\x00" + ticket.Phase
				row := phases[key]
				row.Workflow = ticket.Workflow
				row.Phase = ticket.Phase
				row.Title = ticket.Phase
				if phase, ok := phaseByID(st.Workflows[ticket.Workflow], ticket.Phase); ok {
					row.Title = phase.Title
				}
				row.Count++
				phases[key] = row
			}
		}
		history := st.History[ticket.ID]
		if len(history) == 0 {
			out.HistoryStatus = "partial"
			continue
		}
		for index, revision := range history {
			if revision.At.After(now) {
				continue
			}
			if revision.Snapshot.Status == "Done" && (index == 0 || history[index-1].Snapshot.Status != "Done") {
				done[ticket.ID] = revision.At
			}
			start, end := revision.At, now
			if index+1 < len(history) {
				end = history[index+1].At
			}
			if start.Before(since) {
				start = since
			}
			if end.After(now) {
				end = now
			}
			if !end.After(start) {
				continue
			}
			seconds := end.Sub(start).Seconds()
			if revision.Snapshot.Status == "Doing" {
				out.ProgressSeconds += seconds
			}
			if revision.Snapshot.Attention != "" && revision.Snapshot.Status != "Done" && revision.Snapshot.Status != "Cancelled" {
				out.AttentionWaitSeconds += seconds
			}
		}
		if ticket.Status != "Done" {
			delete(done, ticket.ID)
		}
	}
	for id, at := range done {
		if !at.Before(since) {
			out.CompletedInPeriod++
			out.Recent = append(out.Recent, id)
		}
	}
	sort.Slice(out.Recent, func(i, j int) bool {
		a, b := done[out.Recent[i]], done[out.Recent[j]]
		if a.Equal(b) {
			return out.Recent[i] > out.Recent[j]
		}
		return a.After(b)
	})
	if len(out.Recent) > 8 {
		out.Recent = out.Recent[:8]
	}
	out.TicketsPerDay = float64(out.CompletedInPeriod) / float64(in.Days)
	out.TicketsPerHour = out.TicketsPerDay / 24
	for _, row := range phases {
		out.Phases = append(out.Phases, row)
	}
	sort.Slice(out.Phases, func(i, j int) bool {
		a, b := out.Phases[i], out.Phases[j]
		if a.Workflow == b.Workflow {
			return a.Phase < b.Phase
		}
		return a.Workflow < b.Workflow
	})
	return out, nil
}
