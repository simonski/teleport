package app

import (
	"encoding/json"
	"time"

	"teleport/internal/model"
)

type AgreementBasis struct {
	CurrentBasis string `json:"currentBasis"`
	SavedBasis   string `json:"savedBasis"`
	Stale        bool   `json:"stale"`
	Criteria     string `json:"criteria"`
}
type MarkReadyInput struct {
	ID              int64  `json:"id"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Basis           string `json:"basis"`
	Confirm         bool   `json:"confirm"`
}

func completionBasis(st *model.State, t model.Ticket, a model.Account, criteria string, basisVersion int64) model.Agreement {
	versions := map[int64]int64{}
	for _, id := range effectiveDependencies(st, t) {
		versions[id] = st.Tickets[id].Version
	}
	agreement := model.Agreement{Kind: "Done", Actor: a.ID, At: time.Now().UTC(), Criteria: criteria, BasisVersion: basisVersion, Basis: executionBasis(st, t), RecordVersion: t.Version + 1, ProjectID: t.ProjectID, Workflow: t.Workflow, PrerequisiteVersions: versions}
	if t.Delivery != nil && t.Delivery.Candidate != nil {
		agreement.Commit = t.Delivery.Candidate.Commit
		agreement.Base = t.Delivery.Candidate.Base
	}
	return agreement
}

func agreementOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	switch operation {
	case "getTicketAgreements":
		var in IDInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Viewer")
		if err != nil {
			return nil, false, err
		}
		current, saved, criteria := executionBasis(st, t), "", t.Criteria
		if t.Refinement != nil {
			saved = t.Refinement.AcceptedBasis
			if saved == "" {
				saved = t.Refinement.Basis
			}
			criteria = t.Refinement.Criteria
		}
		return AgreementBasis{CurrentBasis: current, SavedBasis: saved, Stale: current != saved, Criteria: criteria}, false, nil
	case "markReady":
		var in MarkReadyInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		if err = expected(t, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if !in.Confirm || t.Status != "Ready" || !t.Agent || busy(t) || hasContinuation(t) || t.Refinement == nil || t.Refinement.AcceptedBy == "" || in.Basis != executionBasis(st, t) {
			return nil, false, failure(409, "agreement_stale", "Confirm exact current readiness for previously accepted, stopped Ready work")
		}
		t.Refinement.AcceptedBasis = in.Basis
		t.Refinement.AcceptedBy = a.ID
		t.Refinement.AcceptedAt = time.Now().UTC()
		agreement := completionBasis(st, t, a, t.Refinement.Criteria, t.Version)
		agreement.Kind = "Ready"
		t.Agreements = append(t.Agreements, agreement)
		saveTicket(st, &t, a, operation)
		return t, true, nil
	case "rejectRefinement":
		var in VersionInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		t, err := ticketFor(st, a, in.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		if err = expected(t, in.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if t.Status != "Refine" || t.Refinement == nil || busy(t) || t.Question != nil && !t.Question.Resolved && !t.Question.Obsolete {
			return nil, false, failure(409, "proposal_unavailable", "Select a stopped refinement proposal without an open question")
		}
		if _, err = applyRoute(&t, st.Workflows[t.Workflow], "failure", nil); err != nil {
			return nil, false, err
		}
		t.Refinement = nil
		t.ResumeAttempt = ""
		t.Attention = "Refinement rejected: " + in.Reason
		saveTicket(st, &t, a, operation)
		return t, true, nil
	}
	return ideaOperations(st, a, operation, raw)
}
