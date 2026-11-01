package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"teleport/internal/model"
)

type SaveIdeaInput struct {
	ID              string `json:"id,omitempty"`
	ExpectedVersion int64  `json:"expectedVersion,omitempty"`
	ProjectID       int64  `json:"projectId,omitempty"`
	Title           string `json:"title"`
	Text            string `json:"text"`
	RequestID       string `json:"requestId,omitempty"`
}
type IdeaInput struct {
	ID     string `json:"id"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type ListIdeasInput struct {
	ProjectID     int64  `json:"projectId,omitempty"`
	InboxOnly     bool   `json:"inboxOnly,omitempty"`
	Query         string `json:"query,omitempty"`
	IncludeHidden bool   `json:"includeHidden,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}
type ProposeIdeaInput struct {
	ID              string             `json:"id"`
	ExpectedVersion int64              `json:"expectedVersion"`
	RequestID       string             `json:"requestId"`
	Targets         []model.IdeaTarget `json:"targets"`
}
type AcceptIdeaInput struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	ProposalID      string `json:"proposalId"`
}
type IdeaProposalInput struct {
	ID         string `json:"id"`
	ProposalID string `json:"proposalId"`
}
type IdeaProposalDetail struct {
	Proposal   model.IdeaProposal    `json:"proposal"`
	Acceptance *model.IdeaAcceptance `json:"acceptance,omitempty"`
}
type IdeaControlInput struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	ProjectID       int64  `json:"projectId"`
	Confirm         bool   `json:"confirm,omitempty"`
	Archived        *bool  `json:"archived,omitempty"`
	Deleted         *bool  `json:"deleted,omitempty"`
}
type IdeaDetail struct {
	Idea        model.Idea             `json:"idea"`
	Proposals   []model.IdeaProposal   `json:"proposals"`
	Acceptances []model.IdeaAcceptance `json:"acceptances"`
	Links       []model.IdeaLink       `json:"links"`
	HasOlder    bool                   `json:"hasOlder"`
}
type IdeaPlacementPreview struct {
	ID        string `json:"id"`
	Version   int64  `json:"version"`
	ProjectID int64  `json:"projectId"`
	Warning   string `json:"warning"`
}

func ideaCollections(st *model.State) {
	if st.Ideas == nil {
		st.Ideas = map[string]model.Idea{}
	}
	if st.IdeaHistory == nil {
		st.IdeaHistory = map[string][]model.IdeaRevision{}
	}
	if st.IdeaProposals == nil {
		st.IdeaProposals = map[string]model.IdeaProposal{}
	}
	if st.IdeaAcceptances == nil {
		st.IdeaAcceptances = map[string]model.IdeaAcceptance{}
	}
}
func ideaFor(st *model.State, a model.Account, id, minimum string) (model.Idea, error) {
	i, ok := st.Ideas[id]
	if !ok || i.Placement == "inbox" && i.Creator != a.ID && !a.Administrator || i.Placement == "project" && role(st, a, i.ProjectID) == "" {
		return model.Idea{}, failure(404, "not_found", "Idea not found")
	}
	if i.Placement == "project" {
		if err := require(st, a, i.ProjectID, minimum); err != nil {
			return model.Idea{}, err
		}
	}
	return i, nil
}
func ideaExpected(i model.Idea, v int64) error {
	if v < 1 || i.Version != v {
		return failure(409, "version_conflict", "Idea changed; refresh before retrying")
	}
	return nil
}
func ideaEvent(st *model.State, i model.Idea, a model.Account, action, proposalID string) {
	events := st.IdeaHistory[i.ID]
	st.IdeaHistory[i.ID] = append(events, model.IdeaRevision{Sequence: int64(len(events) + 1), TransactionID: st.Revision + 1, Actor: a.ID, Action: action, At: time.Now().UTC(), Snapshot: i, ProposalID: proposalID})
}
func saveIdea(st *model.State, i *model.Idea, a model.Account, action string) {
	i.Version++
	i.UpdatedAt = time.Now().UTC()
	if i.CreatedAt.IsZero() {
		i.CreatedAt = i.UpdatedAt
	}
	st.Ideas[i.ID] = *i
	ideaEvent(st, *i, a, action, "")
}
func ideaTargetProject(st *model.State, a model.Account, target model.IdeaTarget, minimum string) (int64, error) {
	if target.Unavailable || (target.Create == nil) == (target.ExistingID == 0) {
		return 0, failure(422, "invalid_target", "Choose exactly one create or existing target")
	}
	project := int64(0)
	if target.Create != nil {
		project = target.Create.ProjectID
	} else {
		t, err := ticketFor(st, a, target.ExistingID, minimum)
		if err != nil {
			return 0, err
		}
		project = t.ProjectID
	}
	if err := require(st, a, project, minimum); err != nil {
		return 0, err
	}
	return project, nil
}
func ideaDetail(st *model.State, a model.Account, i model.Idea) IdeaDetail {
	d := IdeaDetail{Idea: i, Proposals: []model.IdeaProposal{}, Acceptances: []model.IdeaAcceptance{}, Links: []model.IdeaLink{}}
	for _, p := range st.IdeaProposals {
		if p.IdeaID != i.ID {
			continue
		}
		p.Targets = append([]model.IdeaTarget{}, p.Targets...)
		for n, target := range p.Targets {
			if _, err := ideaTargetProject(st, a, target, "Viewer"); err != nil {
				p.Targets[n] = model.IdeaTarget{Unavailable: true}
			}
		}
		d.Proposals = append(d.Proposals, p)
	}
	sort.Slice(d.Proposals, func(x, y int) bool { return d.Proposals[x].At.Before(d.Proposals[y].At) })
	for _, acceptance := range st.IdeaAcceptances {
		if acceptance.IdeaID != i.ID {
			continue
		}
		acceptance.TicketIDs = append([]int64{}, acceptance.TicketIDs...)
		ids := []int64{}
		for _, id := range acceptance.TicketIDs {
			if _, err := ticketFor(st, a, id, "Viewer"); err == nil {
				ids = append(ids, id)
			}
		}
		acceptance.TicketIDs = ids
		d.Acceptances = append(d.Acceptances, acceptance)
	}
	sort.Slice(d.Acceptances, func(x, y int) bool { return d.Acceptances[x].At.Before(d.Acceptances[y].At) })
	for _, link := range st.IdeaLinks {
		if link.IdeaID == i.ID {
			if _, err := ticketFor(st, a, link.TicketID, "Viewer"); err == nil {
				d.Links = append(d.Links, link)
			}
		}
	}
	if len(d.Proposals) > 20 {
		d.Proposals = d.Proposals[len(d.Proposals)-20:]
		d.HasOlder = true
	}
	if len(d.Acceptances) > 20 {
		d.Acceptances = d.Acceptances[len(d.Acceptances)-20:]
		d.HasOlder = true
	}
	if len(d.Links) > 200 {
		d.Links = d.Links[len(d.Links)-200:]
		d.HasOlder = true
	}
	return d
}
func ideaOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	ideaCollections(st)
	switch operation {
	case "saveIdea":
		var in SaveIdeaInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		in.Title = strings.TrimSpace(in.Title)
		if in.Title == "" || len(in.Title) > 500 || len(in.Text) > 20000 {
			return nil, false, failure(422, "invalid_idea", "Use a title up to 500 bytes and text up to 20,000 bytes")
		}
		if in.ID == "" {
			if in.ProjectID != 0 {
				if err := require(st, a, in.ProjectID, "Contributor"); err != nil {
					return nil, false, err
				}
			}
			key, fingerprint, previous, err := receipt(st, a, operation, in.RequestID, in)
			if err != nil {
				return nil, false, err
			}
			if previous != nil {
				var old model.Idea
				json.Unmarshal(previous, &old)
				current, err := ideaFor(st, a, old.ID, "Contributor")
				return current, false, err
			}
			st.NextIdeaID++
			i := model.Idea{ID: fmt.Sprintf("idea:%d", st.NextIdeaID), Title: in.Title, Text: in.Text, Creator: a.ID, ProjectID: in.ProjectID, Placement: "inbox"}
			if in.ProjectID != 0 {
				i.Placement = "project"
			}
			saveIdea(st, &i, a, operation)
			remember(st, key, fingerprint, i)
			return i, true, nil
		}
		i, err := ideaFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		if err = ideaExpected(i, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if in.ProjectID != 0 && in.ProjectID != i.ProjectID {
			return nil, false, failure(409, "use_placement", "Change placement through its explicit publication control")
		}
		if i.Title == in.Title && i.Text == in.Text {
			return i, false, nil
		}
		i.Title, i.Text = in.Title, in.Text
		saveIdea(st, &i, a, operation)
		return i, true, nil
	case "listIdeas":
		var in ListIdeasInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, false, err
		}
		if in.ProjectID != 0 {
			if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
				return nil, false, err
			}
		}
		items := []model.Idea{}
		for _, i := range st.Ideas {
			n, _ := strconv.ParseInt(strings.TrimPrefix(i.ID, "idea:"), 10, 64)
			if n <= after || in.ProjectID != 0 && i.ProjectID != in.ProjectID || in.InboxOnly && i.Placement != "inbox" || !in.IncludeHidden && (i.Archived || i.Deleted) {
				continue
			}
			if _, err := ideaFor(st, a, i.ID, "Viewer"); err != nil {
				continue
			}
			match := true
			for _, token := range strings.Fields(strings.ToLower(in.Query)) {
				if !strings.Contains(strings.ToLower(i.Title+" "+i.Text), token) {
					match = false
				}
			}
			if match {
				items = append(items, i)
			}
		}
		number := func(i model.Idea) int64 {
			n, _ := strconv.ParseInt(strings.TrimPrefix(i.ID, "idea:"), 10, 64)
			return n
		}
		sort.Slice(items, func(x, y int) bool { return number(items[x]) < number(items[y]) })
		result := Page[model.Idea]{Items: items}
		if len(items) > limit {
			result.Items = items[:limit]
			result.NextCursor = cursor(number(items[limit-1]))
		}
		return result, false, nil
	case "getIdea", "getIdeaHistory":
		var in IdeaInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		i, err := ideaFor(st, a, in.ID, "Viewer")
		if err != nil {
			return nil, false, err
		}
		if operation == "getIdea" {
			return ideaDetail(st, a, i), false, nil
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, false, err
		}
		result := Page[model.IdeaRevision]{Items: []model.IdeaRevision{}}
		for _, event := range st.IdeaHistory[i.ID] {
			if event.Sequence <= after {
				continue
			}
			if len(result.Items) == limit {
				result.NextCursor = cursor(result.Items[len(result.Items)-1].Sequence)
				break
			}
			result.Items = append(result.Items, event)
		}
		return result, false, nil
	case "getIdeaProposal":
		var in IdeaProposalInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		i, err := ideaFor(st, a, in.ID, "Viewer")
		if err != nil {
			return nil, false, err
		}
		p, ok := st.IdeaProposals[in.ProposalID]
		if !ok || p.IdeaID != i.ID {
			return nil, false, failure(404, "not_found", "Proposal not found")
		}
		p.Targets = append([]model.IdeaTarget{}, p.Targets...)
		for n, target := range p.Targets {
			if _, err := ideaTargetProject(st, a, target, "Viewer"); err != nil {
				p.Targets[n] = model.IdeaTarget{Unavailable: true}
			}
		}
		result := IdeaProposalDetail{Proposal: p}
		if acceptance, ok := st.IdeaAcceptances[p.ID]; ok {
			ids := []int64{}
			for _, id := range acceptance.TicketIDs {
				if _, err := ticketFor(st, a, id, "Viewer"); err == nil {
					ids = append(ids, id)
				}
			}
			acceptance.TicketIDs = ids
			result.Acceptance = &acceptance
		}
		return result, false, nil
	case "getTicketIdeas", "getIdeaLinks":
		var in IDInput
		ideaID := ""
		if operation == "getIdeaLinks" {
			var source IdeaInput
			if err := parse(raw, &source); err != nil {
				return nil, false, err
			}
			if _, err := ideaFor(st, a, source.ID, "Viewer"); err != nil {
				return nil, false, err
			}
			ideaID = source.ID
			in.Cursor, in.Limit = source.Cursor, source.Limit
		} else {
			if err := parse(raw, &in); err != nil {
				return nil, false, err
			}
			if _, err := ticketFor(st, a, in.ID, "Viewer"); err != nil {
				return nil, false, err
			}
		}
		after, limit, err := pageBounds(in.Cursor, in.Limit)
		if err != nil {
			return nil, false, err
		}
		result := Page[model.IdeaLink]{Items: []model.IdeaLink{}}
		last := int64(0)
		for n, link := range st.IdeaLinks {
			sequence := int64(n + 1)
			if sequence <= after || ideaID != "" && link.IdeaID != ideaID || ideaID == "" && link.TicketID != in.ID {
				continue
			}
			if _, err := ideaFor(st, a, link.IdeaID, "Viewer"); err != nil {
				continue
			}
			if _, err := ticketFor(st, a, link.TicketID, "Viewer"); err != nil {
				continue
			}
			if len(result.Items) == limit {
				result.NextCursor = cursor(last)
				break
			}
			result.Items = append(result.Items, link)
			last = sequence
		}
		return result, false, nil
	case "submitIdeaProposal":
		if len(raw) > 96*1024 {
			return nil, false, failure(413, "proposal_too_large", "Use a proposal within 96 KiB")
		}
		var in ProposeIdeaInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		i, err := ideaFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		key, fingerprint, previous, err := receipt(st, a, operation, in.RequestID, in)
		if err != nil {
			return nil, false, err
		}
		if previous != nil {
			var p model.IdeaProposal
			json.Unmarshal(previous, &p)
			for _, target := range p.Targets {
				if _, err := ideaTargetProject(st, a, target, "Viewer"); err != nil {
					return nil, false, err
				}
			}
			return p, false, nil
		}
		if err = ideaExpected(i, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if i.Archived || i.Deleted || len(in.Targets) < 1 || len(in.Targets) > 20 {
			return nil, false, failure(422, "invalid_proposal", "Use a visible Idea and 1–20 authorized targets")
		}
		for _, target := range in.Targets {
			if _, err = ideaTargetProject(st, a, target, "Viewer"); err != nil {
				return nil, false, err
			}
			if target.Create == nil && target.ExistingVersion < 1 {
				return nil, false, failure(422, "invalid_target", "Existing targets require an exact version")
			}
		}
		p := model.IdeaProposal{ID: "proposal:" + randomID()[:24], Version: 1, IdeaID: i.ID, IdeaVersion: i.Version, Actor: a.ID, At: time.Now().UTC(), Targets: in.Targets}
		st.IdeaProposals[p.ID] = p
		ideaEvent(st, i, a, operation, p.ID)
		remember(st, key, fingerprint, p)
		return p, true, nil
	case "acceptIdeaProposal":
		var in AcceptIdeaInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		i, err := ideaFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		p, ok := st.IdeaProposals[in.ProposalID]
		if !ok || p.IdeaID != i.ID {
			return nil, false, failure(404, "not_found", "Proposal not found")
		}
		if i.Archived || i.Deleted {
			return nil, false, failure(409, "idea_hidden", "Restore this Idea before acceptance")
		}
		if accepted, ok := st.IdeaAcceptances[p.ID]; ok {
			for _, id := range accepted.TicketIDs {
				if _, err := ticketFor(st, a, id, "Contributor"); err != nil {
					return nil, false, err
				}
			}
			return accepted, false, nil
		}
		if err = ideaExpected(i, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if p.IdeaVersion != i.Version {
			return nil, false, failure(409, "proposal_stale", "Refresh the proposal against the current Idea")
		}
		accepted := model.IdeaAcceptance{IdeaID: i.ID, IdeaVersion: i.Version, ProposalID: p.ID, ProposalVersion: p.Version, Actor: a.ID, At: time.Now().UTC(), TicketIDs: []int64{}}
		for _, target := range p.Targets {
			if _, err = ideaTargetProject(st, a, target, "Contributor"); err != nil {
				return nil, false, err
			}
			var ticket model.Ticket
			created := target.Create != nil
			if created {
				c := target.Create
				// Acceptance is atomically fenced by proposal identity. Never borrow a
				// guessable receipt from the public ticket-creation namespace.
				input := CreateTicketInput{ProjectID: c.ProjectID, ParentID: c.ParentID, Kind: c.Kind, Title: c.Title, Description: c.Description, Criteria: c.Criteria, Agent: c.Agent, RequestID: randomID()}
				body, _ := json.Marshal(input)
				result, _, err := projectOperations(st, a, "createTicket", body)
				if err != nil {
					return nil, false, err
				}
				ticket = result.(model.Ticket)
			} else {
				ticket = st.Tickets[target.ExistingID]
				if err = expected(ticket, target.ExistingVersion); err != nil {
					return nil, false, err
				}
			}
			accepted.TicketIDs = append(accepted.TicketIDs, ticket.ID)
			st.IdeaLinks = append(st.IdeaLinks, model.IdeaLink{IdeaID: i.ID, IdeaVersion: p.IdeaVersion, ProposalID: p.ID, ProposalVersion: p.Version, TicketID: ticket.ID, TicketVersion: ticket.Version, Created: created, Actor: a.ID, At: accepted.At})
		}
		st.IdeaAcceptances[p.ID] = accepted
		i.AcceptedCount++
		saveIdea(st, &i, a, operation)
		return accepted, true, nil
	case "previewIdeaPlacement", "setIdeaPlacement", "setIdeaVisibility":
		var in IdeaControlInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		i, err := ideaFor(st, a, in.ID, "Maintainer")
		if err != nil {
			return nil, false, err
		}
		if err = ideaExpected(i, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if i.CurationState != "" && !contains([]string{"stopped", "cancelled", "complete"}, i.CurationState) {
			return nil, false, failure(409, "curation_pending", "Separately stop curation and confirm its owned exit first")
		}
		if operation == "setIdeaVisibility" {
			if in.Archived == nil && in.Deleted == nil {
				return nil, false, failure(422, "invalid_visibility", "Select archive or delete state")
			}
			old := i
			if in.Archived != nil {
				i.Archived = *in.Archived
			}
			if in.Deleted != nil {
				i.Deleted = *in.Deleted
			}
			if i == old {
				return i, false, nil
			}
			saveIdea(st, &i, a, operation)
			return i, true, nil
		}
		warning := "The Idea and retained history become private to its creator and administrators."
		if in.ProjectID != 0 {
			if err = require(st, a, in.ProjectID, "Contributor"); err != nil {
				return nil, false, err
			}
			warning = "Publish this Idea and its retained text, history and proposals to all authorized destination-project members."
		} else if i.Creator != a.ID && !a.Administrator {
			return nil, false, failure(403, "forbidden", "Only the creator with source Maintainer rights or administrator can return an Idea to its inbox")
		}
		if operation == "previewIdeaPlacement" {
			return IdeaPlacementPreview{ID: i.ID, Version: i.Version, ProjectID: in.ProjectID, Warning: warning}, false, nil
		}
		if !in.Confirm {
			return nil, false, failure(409, "confirmation_required", warning)
		}
		if i.ProjectID == in.ProjectID {
			return i, false, nil
		}
		i.ProjectID = in.ProjectID
		i.Placement = "inbox"
		if in.ProjectID != 0 {
			i.Placement = "project"
		}
		saveIdea(st, &i, a, operation)
		return i, true, nil
	}
	return presentationOperations(st, a, operation, raw)
}
