package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"teleport/internal/localruntime"
	"teleport/internal/model"
)

type ProjectDirectoryInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Query  string `json:"query,omitempty"`
}

type ProjectSummary struct {
	Simulation     bool       `json:"simulation,omitempty"`
	ID             int64      `json:"id"`
	Version        int64      `json:"version"`
	Key            string     `json:"key"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Tickets        int        `json:"tickets"`
	Epics          int        `json:"epics"`
	Goals          int        `json:"goals"`
	Tokens         *int64     `json:"tokens"`
	UsageStatus    string     `json:"usageStatus"`
	Agents         *int       `json:"agents"`
	AgentsScope    string     `json:"agentsScope"`
	LastActive     *time.Time `json:"lastActive"`
	ActivityStatus string     `json:"activityStatus"`
}

type SaveProjectInput struct {
	ID              int64   `json:"id"`
	ExpectedVersion int64   `json:"expectedVersion"`
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	Root            *string `json:"root,omitempty"`
}

func saveProject(st *model.State, a model.Account, raw json.RawMessage) (any, bool, error) {
	var in SaveProjectInput
	if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	if err := require(st, a, in.ID, "Maintainer"); err != nil {
		return nil, false, err
	}
	p := st.Projects[in.ID]
	if p.Version != in.ExpectedVersion {
		return nil, false, failure(409, "version_conflict", "Project changed; refresh before saving")
	}
	name, description, root := p.Name, p.Description, p.Root
	if root == "" && p.Repository != nil {
		root = p.Repository.Repository
	}
	if root == "" {
		root = defaultProjectRoot
	}
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		description = *in.Description
	}
	if in.Root != nil {
		root = strings.TrimSpace(*in.Root)
		if !validProjectRoot(root) {
			return nil, false, failure(422, "invalid_project_root", "Use an absolute project root or a path starting with ~/")
		}
		if p.Repository != nil && root != p.Root && root != p.Repository.Repository {
			return nil, false, failure(409, "binding_required", "Change a granted project root through the administrator binding")
		}
	}
	if name == "" || len(name) > 200 || len(description) > 20000 {
		return nil, false, failure(422, "invalid_project", "Provide a name within 200 bytes and description within 20000 bytes")
	}
	for id, other := range st.Projects {
		if id != p.ID && sameProjectName(other.Name, name) {
			return nil, false, failure(409, "project_name_exists", "Project name already exists")
		}
	}
	if name == p.Name && description == p.Description && (in.Root == nil || root == p.Root) {
		return p, false, nil
	}
	p.Name, p.Description, p.Version = name, description, p.Version+1
	if in.Root != nil {
		p.Root = root
	}
	st.Projects[p.ID] = p
	return p, true, nil
}

func (s *Service) projectDirectory(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in ProjectDirectoryInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	after, limit, err := pageBounds(in.Cursor, in.Limit)
	if err != nil {
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
	observed := map[string]localruntime.Status{}
	if s.localClient != nil {
		for _, row := range st.LocalSessions {
			if activeLocal(row) && localAllowed(st, a, row) == nil {
				out, callErr := s.localClient.Call(ctx, localruntime.Request{Operation: "list"})
				if callErr == nil {
					for _, status := range out.Items {
						observed[status.ID] = status
					}
				}
				break
			}
		}
	}
	// A runtime observation grants no enduring account/project access.
	st, err = s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err = directoryActor(st, token)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for id, p := range st.Projects {
		if id > after && role(st, a, id) != "" && strings.Contains(strings.ToLower(p.Name), strings.ToLower(in.Query)) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	page := Page[ProjectSummary]{Items: []ProjectSummary{}}
	if len(ids) > limit {
		page.NextCursor = cursor(ids[limit-1])
		ids = ids[:limit]
	}
	rows := map[int64]*ProjectSummary{}
	for _, id := range ids {
		p := st.Projects[id]
		count := 0
		rows[id] = &ProjectSummary{Simulation: p.Simulation, ID: id, Version: p.Version, Key: p.Key, Name: p.Name, Description: p.Description, Agents: &count, AgentsScope: "own-authorized", UsageStatus: "unavailable", ActivityStatus: "retained-events"}
	}
	for _, ticket := range st.Tickets {
		row := rows[ticket.ProjectID]
		if row == nil || ticket.Protected || ticket.Archived || ticket.Deleted {
			continue
		}
		row.Tickets++
		if ticket.Kind == "epic" {
			row.Epics++
		} else if ticket.Kind == "goal" {
			row.Goals++
		}
	}
	for _, revisions := range st.History {
		for _, revision := range revisions {
			if row := rows[revision.Snapshot.ProjectID]; row != nil && !revision.Snapshot.Protected {
				row.observeActivity(revision.At)
			}
		}
	}
	for _, session := range st.LocalSessions {
		row := rows[session.ProjectID]
		if row == nil || localAllowed(st, a, session) != nil {
			continue
		}
		row.observeActivity(session.CreatedAt)
		if session.ExitedAt != nil {
			row.observeActivity(*session.ExitedAt)
		}
		if !activeLocal(session) || session.StopRequested || session.State == "stopping" {
			continue
		}
		status, ok := observed[session.ID]
		if !ok || status.LaunchID != session.LaunchID || status.Generation != session.Generation || session.State != "running" || status.State == "unknown" {
			row.Agents = nil
			continue
		}
		row.observeActivity(status.StartedAt)
		if status.ExitedAt != nil {
			row.observeActivity(*status.ExitedAt)
		}
		if status.State == "running" && row.Agents != nil {
			*row.Agents++
		}
	}
	for _, id := range ids {
		page.Items = append(page.Items, *rows[id])
	}
	return page, nil
}

func directoryActor(st *model.State, token string) (model.Account, error) {
	a, err := actor(st, token)
	if err == nil && (a.ServiceRunID != "" || a.ServiceSessionID != "") {
		err = failure(403, "service_scope", "Scoped execution credentials cannot read project aggregates")
	}
	return a, err
}

func (row *ProjectSummary) observeActivity(at time.Time) {
	if !at.IsZero() && (row.LastActive == nil || at.After(*row.LastActive)) {
		row.LastActive = &at
	}
}
