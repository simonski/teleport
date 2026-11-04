package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"teleport/internal/model"
)

type UsageBreakdownInput struct {
	ProjectID int64  `json:"projectId,omitempty"`
	GroupBy   string `json:"groupBy"`
	Window    string `json:"window,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}
type UsageBreakdownItem struct {
	Key   string      `json:"key"`
	Label string      `json:"label"`
	Usage UsageWindow `json:"usage"`
}
type UsageBreakdownPage struct {
	Items      []UsageBreakdownItem `json:"items"`
	NextCursor string               `json:"nextCursor"`
	AsOf       time.Time            `json:"asOf"`
}

func usageGroup(st *model.State, a model.Account, row model.LocalSession, group string) (string, string) {
	if group != "project" && row.Managed == nil && row.Spellcheck == nil && row.Creator != a.ID {
		return "private", "Private sessions"
	}
	at := retainedAttribution(row)
	switch group {
	case "user_agent":
		userKey, userName := usageGroup(st, a, row, "user")
		agentKey, agentName := usageGroup(st, a, row, "agent")
		return base64.RawURLEncoding.EncodeToString([]byte(userKey + "\x00" + agentKey)), userName + " / " + agentName
	case "project":
		p := st.Projects[at.ProjectID]
		return fmt.Sprintf("project:%020d", at.ProjectID), p.Name
	case "user":
		if at.InitiatorID == "" {
			return "unknown", "System or unknown initiator"
		}
		name := st.Accounts[at.InitiatorID].Username
		if name == "" {
			name = at.InitiatorID
		}
		return "user:" + at.InitiatorID, name
	case "agent":
		if at.AgentID != "" {
			definition, ok := st.AgentDefinitions[at.AgentID]
			if !a.Administrator && (!ok || definition.Archived) {
				return "restricted", "Unavailable agent"
			}
			name := definition.Name
			if name == "" {
				name = at.AgentName
			}
			if name == "" {
				name = "Unknown agent"
			}
			return "agent:" + at.AgentID, name
		}
		if row.Managed != nil {
			return "workflow", "Managed workflow"
		}
		return "unassigned", "No configured agent"
	case "epic", "goal":
		id := at.EpicID
		if group == "goal" {
			id = at.GoalID
		}
		if id == 0 {
			if at.HierarchyStatus == "not_applicable" {
				return "not-applicable", "No ticket context"
			}
			return "unknown", "Unknown historical context"
		}
		ticket, ok := st.Tickets[id]
		if !ok || role(st, a, ticket.ProjectID) == "" {
			return "restricted", "Restricted or moved context"
		}
		return fmt.Sprintf("%s:%020d", group, id), fmt.Sprintf("#%d · %s", id, ticket.Title)
	}
	return "unknown", "Unknown"
}

func (s *Service) usageBreakdown(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in UsageBreakdownInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	if !contains([]string{"project", "user", "agent", "user_agent", "epic", "goal"}, in.GroupBy) {
		return nil, failure(422, "invalid_group", "Choose project, user, agent, user and agent, epic or goal")
	}
	if in.Window == "" {
		in.Window = "month"
	}
	durations := map[string]time.Duration{"hour": time.Hour, "day": 24 * time.Hour, "week": 7 * 24 * time.Hour, "month": 30 * 24 * time.Hour, "all": 0}
	duration, ok := durations[in.Window]
	if !ok {
		return nil, failure(422, "invalid_period", "Choose hour, day, week, month or all")
	}
	_, limit, err := pageBounds("", in.Limit)
	if err != nil {
		return nil, err
	}
	after := ""
	if in.Cursor != "" {
		data, e := base64.RawURLEncoding.DecodeString(in.Cursor)
		prefix := "usage:" + in.GroupBy + ":" + in.Window + ":"
		if e != nil || !strings.HasPrefix(string(data), prefix) {
			return nil, failure(400, "invalid_cursor", "Invalid usage cursor")
		}
		after = strings.TrimPrefix(string(data), prefix)
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
	since := time.Time{}
	if duration != 0 {
		since = now.Add(-duration)
	}
	type bucket struct {
		label    string
		sessions map[string]model.LocalSession
	}
	buckets := map[string]*bucket{}
	for id, row := range st.LocalSessions {
		if row.Harness == "shell" || role(st, a, row.ProjectID) == "" || in.ProjectID != 0 && row.ProjectID != in.ProjectID || row.CreatedAt.After(now) || !since.IsZero() && row.CreatedAt.Before(since) {
			continue
		}
		key, label := usageGroup(st, a, row, in.GroupBy)
		if key <= after {
			continue
		}
		b := buckets[key]
		if b == nil {
			b = &bucket{label: label, sessions: map[string]model.LocalSession{}}
			buckets[key] = b
		}
		b.sessions[id] = row
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := UsageBreakdownPage{Items: []UsageBreakdownItem{}, AsOf: now}
	for i, key := range keys {
		if i == limit {
			out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte("usage:" + in.GroupBy + ":" + in.Window + ":" + keys[i-1]))
			break
		}
		copyState := *st
		copyState.LocalSessions = buckets[key].sessions
		out.Items = append(out.Items, UsageBreakdownItem{Key: key, Label: buckets[key].label, Usage: usageWindow(&copyState, a, in.ProjectID, in.Window, since, now)})
	}
	return out, nil
}
