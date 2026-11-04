package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"teleport/internal/gitwork"
	"teleport/internal/localruntime"
	"teleport/internal/model"
)

type SpellcheckInput struct {
	TicketID        int64  `json:"ticketId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	UsageID         string `json:"usageId"`
	UsageVersion    int64  `json:"usageVersion"`
	RequestID       string `json:"requestId"`
}
type SpellcheckView struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}
type SpellcheckStatusInput struct {
	ID string `json:"id"`
}
type SpellcheckMark struct {
	UsageID string `json:"usageId"`
	Current bool   `json:"current"`
	Reason  string `json:"reason,omitempty"`
}
type SpellcheckMarks struct {
	Items []SpellcheckMark `json:"items"`
}

func spellcheckBasis(t model.Ticket) string {
	data, _ := json.Marshal([]string{t.Title, t.Description})
	return digest(string(data))
}
func spellcheckEligible(st *model.State, a model.Account, in SpellcheckInput) (model.Ticket, model.AgentUsage, model.AgentDefinition, error) {
	t, err := ticketFor(st, a, in.TicketID, "Contributor")
	if err != nil {
		return t, model.AgentUsage{}, model.AgentDefinition{}, err
	}
	u, ok := st.AgentUsages[in.UsageID]
	agent := st.AgentDefinitions[u.AgentID]
	if !ok || u.ProjectID != t.ProjectID || !u.Enabled || u.Kind != "todo-spellcheck" || u.Version != in.UsageVersion || agent.ID == "" || agent.Archived {
		return t, u, agent, failure(409, "usage_unavailable", "The configured agent action changed or is unavailable")
	}
	if err = expected(t, in.ExpectedVersion); err != nil {
		return t, u, agent, err
	}
	if a.ServiceRunID != "" || a.ServiceSessionID != "" || t.Kind != "task" || t.Agent || t.Status != "Todo" || t.Protected || t.Archived || t.Deleted || busy(t) || t.StopRequested || t.ResumeHold {
		return t, u, agent, failure(409, "spellcheck_unavailable", "Spellcheck requires an editable human Todo ticket")
	}
	p := st.Projects[t.ProjectID]
	w := st.Workflows[t.Workflow]
	if len(st.Accounts) != 1 || st.Settings.Agents || st.Settings.Handbrake || p.Handbrake || p.LaneLocks[w.ID+"#*"] || p.LaneLocks[w.ID+"#"+t.Phase] || p.Simulation || p.Repository == nil {
		return t, u, agent, failure(409, "admission_blocked", "Spellcheck requires an available single-user local project with execution enabled")
	}
	if !projectAllowsHarness(p, agent.Harness) {
		return t, u, agent, failure(403, "harness_not_permitted", "This harness is not permitted for the project")
	}
	for _, row := range st.LocalSessions {
		if row.Spellcheck != nil && row.Spellcheck.TicketID == t.ID && activeLocal(row) {
			return t, u, agent, failure(409, "spellcheck_running", "A spellcheck is already running for this ticket")
		}
	}
	if err = phaseCapacity(st, t, model.Phase{ID: t.Phase, Role: "Refine"}); err != nil {
		return t, u, agent, err
	}
	return t, u, agent, nil
}

func (s *Service) spellcheckCall(ctx context.Context, token, operation string, raw json.RawMessage) (any, error) {
	if operation == "getTicketSpellchecks" {
		var in IDInput
		if err := parse(raw, &in); err != nil {
			return nil, err
		}
		st, err := s.db.Read(ctx)
		if err != nil {
			return nil, err
		}
		a, err := actor(st, token)
		if err != nil {
			return nil, err
		}
		if a.ServiceRunID != "" || a.ServiceSessionID != "" {
			return nil, failure(403, "human_required", "A human account is required")
		}
		t, err := ticketFor(st, a, in.ID, "Viewer")
		if err != nil {
			return nil, err
		}
		out := SpellcheckMarks{Items: []SpellcheckMark{}}
		for _, v := range t.Spellchecks {
			mark := SpellcheckMark{UsageID: v.UsageID, Current: true}
			usage := st.AgentUsages[v.UsageID]
			agent := st.AgentDefinitions[usage.AgentID]
			if v.Basis != spellcheckBasis(t) {
				mark.Current = false
				mark.Reason = "text_changed"
			} else if usage.Version != v.UsageVersion || agent.ID != v.AgentID || agent.Version != v.AgentVersion || agent.Archived {
				mark.Current = false
				mark.Reason = "configuration_changed"
			}
			out.Items = append(out.Items, mark)
		}
		return out, nil
	}
	if operation == "getSpellcheckRun" {
		var in SpellcheckStatusInput
		if err := parse(raw, &in); err != nil {
			return nil, err
		}
		st, err := s.db.Read(ctx)
		if err != nil {
			return nil, err
		}
		a, err := actor(st, token)
		if err != nil {
			return nil, err
		}
		if a.ServiceRunID != "" || a.ServiceSessionID != "" {
			return nil, failure(403, "human_required", "A human account is required")
		}
		row, ok := st.LocalSessions[in.ID]
		if !ok || row.Spellcheck == nil {
			return nil, failure(404, "not_found", "Spellcheck run not found")
		}
		if err = localAllowed(st, a, row); err != nil {
			return nil, err
		}
		if row.State == "unknown" {
			return SpellcheckView{ID: row.ID, State: "unknown", Error: "Runtime ownership is unconfirmed; inspect the existing session"}, nil
		}
		return SpellcheckView{ID: row.ID, State: row.Spellcheck.State, Error: row.Spellcheck.Error}, nil
	}
	var in SpellcheckInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	s.localMu.Lock()
	defer s.localMu.Unlock()
	if s.localClient == nil {
		return nil, failure(503, "runtime_unavailable", "Local runtime is unavailable")
	}
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := actor(st, token)
	if err != nil {
		return nil, err
	}
	if a.ServiceRunID != "" || a.ServiceSessionID != "" {
		return nil, failure(403, "human_required", "A human account is required")
	}
	key, hash, saved, err := receipt(st, a, operation, in.RequestID, in)
	if err != nil {
		return nil, err
	}
	var row model.LocalSession
	if saved != nil {
		var id string
		if json.Unmarshal(saved, &id) != nil {
			return nil, failure(500, "invalid_receipt", "Invalid run receipt")
		}
		row = st.LocalSessions[id]
		if row.Spellcheck == nil {
			return nil, failure(409, "run_unavailable", "Saved run is unavailable")
		}
		if err = localAllowed(st, a, row); err != nil {
			return nil, err
		}
		if row.State != "preparing" {
			return SpellcheckView{ID: row.ID, State: row.Spellcheck.State, Error: row.Spellcheck.Error}, nil
		}
	}
	if saved == nil {
		t, u, agent, err := spellcheckEligible(st, a, in)
		if err != nil {
			return nil, err
		}
		path, err := exec.LookPath(agent.Harness)
		if err != nil || !filepath.IsAbs(path) {
			return nil, failure(409, "adapter_unavailable", "The configured harness is unavailable")
		}
		p := st.Projects[t.ProjectID]
		if err = gitwork.VerifyCheckout(ctx, p.Repository.Repository); err != nil {
			return nil, failure(409, "project_root_unavailable", "The project root is unavailable")
		}
		if err = s.localClient.Ensure(ctx); err != nil {
			return nil, failure(503, "runtime_unavailable", "The owned runtime is unavailable")
		}
		ping, err := s.localClient.Call(ctx, localruntime.Request{Operation: "ping"})
		if err != nil {
			return nil, err
		}
		skills := installedAgentSkills()
		paths := []string{}
		for _, name := range agent.Skills {
			if skills[name] == "" {
				return nil, failure(409, "skill_unavailable", "A selected skill is unavailable")
			}
			info, statErr := os.Stat(skills[name])
			if statErr != nil || !info.Mode().IsRegular() || info.Size() > 32000 {
				return nil, failure(409, "skill_unavailable", "A selected skill cannot be read or exceeds the launch limit")
			}
			content, readErr := os.ReadFile(skills[name])
			if readErr != nil || len(content) > 32000 {
				return nil, failure(409, "skill_unavailable", "A selected skill cannot be read or exceeds the launch limit")
			}
			paths = append(paths, string(content))
		}
		payload, _ := json.Marshal(struct {
			Agent       model.AgentDefinition `json:"agent"`
			Skills      []string              `json:"skills"`
			Title       string                `json:"title"`
			Description string                `json:"description"`
		}{agent, paths, t.Title, t.Description})
		if len(payload) > 96<<10 {
			return nil, failure(422, "prompt_too_large", "Ticket and agent context exceed the launch limit")
		}
		err = s.db.Update(ctx, func(st *model.State) (bool, error) {
			collections(st)
			if st.LocalSessions == nil {
				st.LocalSessions = map[string]model.LocalSession{}
			}
			current, err := actor(st, token)
			if err != nil {
				return false, err
			}
			t2, u2, agent2, err := spellcheckEligible(st, current, in)
			if err != nil {
				return false, err
			}
			if agent2.Version != agent.Version || u2.Version != u.Version || t2.ProjectID != t.ProjectID || st.Projects[t.ProjectID].Repository.Version != p.Repository.Version {
				return false, failure(409, "launch_basis_changed", "Launch context changed")
			}
			if st.NextLocalSessionID >= 1000 {
				return false, failure(409, "runtime_capacity_exhausted", "Retained session limit reached")
			}
			st.NextLocalSessionID++
			row = model.LocalSession{ID: "H" + strconv.FormatInt(st.NextLocalSessionID, 10), Number: st.NextLocalSessionID, Creator: current.ID, ProjectID: t.ProjectID, Harness: agent.Harness + "-pty", LaunchID: randomID(), Generation: ping.Generation, State: "preparing", BindingVersion: p.Repository.Version, Worktree: model.Worktree{Path: p.Repository.Repository}, CreatedAt: time.Now().UTC(), Persona: &agent, ContextTicketID: t.ID, Spellcheck: &model.SpellcheckRun{UsageID: u.ID, UsageVersion: u.Version, TicketID: t.ID, TicketVersion: t.Version, RequestID: in.RequestID, Payload: string(payload), State: "running"}}
			row.Attribution = freezeAttribution(st, current, t2.ProjectID, t2.ID, &agent2, u2.ID, false)
			st.LocalSessions[row.ID] = row
			remember(st, key, hash, row.ID)
			return true, nil
		})
		if err != nil {
			return nil, err
		}
	}
	path, err := exec.LookPath(row.Persona.Harness)
	if err != nil || !filepath.IsAbs(path) {
		return nil, failure(409, "adapter_unavailable", "The configured harness is unavailable")
	}
	launch := &localruntime.Launch{ID: row.ID, LaunchID: row.LaunchID, Generation: row.Generation, Program: s.localClient.Executable, Args: []string{"_spellcheck-job", path}, Cwd: row.Worktree.Path, Env: map[string]string{"TELEPORT_SPELLCHECK": row.Spellcheck.Payload}, Cols: 100, Rows: 28}
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		cur := st.LocalSessions[row.ID]
		if cur.LaunchID != row.LaunchID || cur.State != "preparing" {
			return false, nil
		}
		currentActor, actorErr := actor(st, token)
		t := st.Tickets[cur.Spellcheck.TicketID]
		p := st.Projects[cur.ProjectID]
		u := st.AgentUsages[cur.Spellcheck.UsageID]
		w := st.Workflows[t.Workflow]
		if actorErr != nil || localAllowed(st, currentActor, cur) != nil || len(st.Accounts) != 1 || st.Settings.Agents || st.Settings.Handbrake || p.Handbrake || p.Repository == nil || p.Repository.Version != cur.BindingVersion || p.LaneLocks[w.ID+"#*"] || p.LaneLocks[w.ID+"#"+t.Phase] || t.Version != cur.Spellcheck.TicketVersion || t.Status != "Todo" || t.Agent || t.Archived || t.Deleted || busy(t) || !u.Enabled || u.Version != cur.Spellcheck.UsageVersion || st.AgentDefinitions[u.AgentID].Version != cur.Persona.Version || !projectAllowsHarness(p, cur.Persona.Harness) {
			return false, failure(409, "launch_basis_changed", "Ticket, configuration or execution authority changed; inspect the existing run")
		}
		out, launchErr := s.localClient.Call(ctx, localruntime.Request{Operation: "launch", Launch: launch})
		if launchErr != nil || out.Status == nil {
			return false, failure(503, "launch_ack_unknown", "Launch outcome is unknown; retry this exact request")
		}
		cur.State = out.Status.State
		cur.ProcessID = out.Status.ProcessID
		cur.ErrorCode = out.Status.ErrorCode
		if cur.State == "failed" {
			cur.Spellcheck.State = "failed"
			cur.Spellcheck.Error = "Harness could not start"
		}
		st.LocalSessions[cur.ID] = cur
		row = cur
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return SpellcheckView{ID: row.ID, State: row.Spellcheck.State, Error: row.Spellcheck.Error}, nil
}

func (s *Service) PumpSpellchecks(ctx context.Context) error {
	if s.localClient == nil {
		return nil
	}
	st, err := s.db.Read(ctx)
	if err != nil {
		return err
	}
	for _, row := range st.LocalSessions {
		if row.Spellcheck == nil || row.Spellcheck.State != "running" || activeLocal(row) {
			continue
		}
		var result struct {
			Title       *string              `json:"title"`
			Description *string              `json:"description"`
			Usage       *model.TokenEvidence `json:"usage,omitempty"`
		}
		observed := row.State == "exited" && row.ErrorCode == "" && row.ExitCode != nil
		valid := false
		if observed {
			request := daemonRequest(row, "read")
			out, e := s.localClient.Call(ctx, request)
			observed = e == nil && out.Status != nil && !out.Gap && out.Next == out.Status.Next && len(out.Data) <= 64<<10
			if observed {
				line := bytes.SplitN(out.Data, []byte("\n"), 2)[0]
				decoder := json.NewDecoder(bytes.NewReader(line))
				decoder.DisallowUnknownFields()
				observed = decoder.Decode(&result) == nil && decoder.Decode(&struct{}{}) == io.EOF
				valid = observed && *row.ExitCode == 0 && !row.StopRequested && result.Title != nil && result.Description != nil && strings.TrimSpace(*result.Title) != "" && len(*result.Title) <= 20000 && len(*result.Description) <= 20000
			}
		}
		err = s.db.Update(ctx, func(st *model.State) (bool, error) {
			current := st.LocalSessions[row.ID]
			if current.Spellcheck == nil || current.Spellcheck.State != "running" {
				return false, nil
			}
			job := current.Spellcheck
			if observed && result.Usage != nil && result.Usage.Input >= 0 && result.Usage.Input <= 3_000_000_000_000 && (result.Usage.Cached == nil || *result.Usage.Cached >= 0 && *result.Usage.Cached <= result.Usage.Input) && result.Usage.Output >= 0 && result.Usage.Output <= 1_000_000_000_000 && (result.Usage.EstimateUSDMicros == nil || *result.Usage.EstimateUSDMicros >= 0 && *result.Usage.EstimateUSDMicros < 1_000_000_000_000_000) && contains([]string{"codex-exec-json", "claude-print-json"}, result.Usage.Source) {
				job.Usage = result.Usage
				applyUsagePrice(st, &current)
			}
			a := st.Accounts[current.Creator]
			t := st.Tickets[job.TicketID]
			p := st.Projects[current.ProjectID]
			u := st.AgentUsages[job.UsageID]
			if !valid || len(st.Accounts) != 1 || a.ID == "" || a.Suspended || require(st, a, current.ProjectID, "Contributor") != nil || p.Repository == nil || p.Repository.Version != current.BindingVersion || !u.Enabled || u.Version != job.UsageVersion || t.ID == 0 || t.ProjectID != current.ProjectID || t.Version != job.TicketVersion || t.Agent || t.Status != "Todo" || t.Protected || t.Deleted || t.Archived || busy(t) || current.StopRequested {
				job.State = "failed"
				job.Error = "Result unavailable or ticket/authority changed; no changes applied"
				job.Payload = ""
				st.LocalSessions[current.ID] = current
				return true, nil
			}
			t.Title = *result.Title
			t.Description = *result.Description
			if err := validContent(t); err != nil {
				job.State = "failed"
				job.Error = "Invalid correction; no changes applied"
				st.LocalSessions[current.ID] = current
				return true, nil
			}
			marks := map[string]model.SpellcheckRecord{}
			for k, v := range t.Spellchecks {
				marks[k] = v
			}
			marks[job.UsageID] = model.SpellcheckRecord{UsageID: job.UsageID, UsageVersion: job.UsageVersion, AgentID: current.Persona.ID, AgentVersion: current.Persona.Version, Actor: a.ID, Basis: spellcheckBasis(t), At: time.Now().UTC()}
			t.Spellchecks = marks
			saveTicket(st, &t, a, "spellcheck")
			job.State = "completed"
			job.Payload = ""
			st.LocalSessions[current.ID] = current
			return true, nil
		})
		if err != nil {
			return fmt.Errorf("spellcheck reconciliation: %w", err)
		}
	}
	return nil
}
