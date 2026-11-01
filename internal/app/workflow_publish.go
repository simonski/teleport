package app

import (
	"encoding/json"

	"teleport/internal/model"
)

type PublishWorkflowInput struct {
	WorkflowKey      string `json:"workflowKey"`
	ExpectedRevision int64  `json:"expectedRevision"`
	RequestID        string `json:"requestId"`
}

func publishWorkflow(st *model.State, a model.Account, raw json.RawMessage) (any, bool, error) {
	var in PublishWorkflowInput
	if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	key, fingerprint, previous, err := receipt(st, a, "publishWorkflow", in.RequestID, in)
	if err != nil {
		return nil, false, err
	}
	if previous != nil {
		var published model.Workflow
		if err := json.Unmarshal(previous, &published); err != nil {
			return nil, false, err
		}
		current, err := workflowRead(st, a, workflowKey(published))
		if err != nil {
			return nil, false, err
		}
		if err = workflowWrite(st, a, current); err != nil {
			return nil, false, err
		}
		return current, false, nil
	}
	w, err := workflowRead(st, a, in.WorkflowKey)
	if err != nil {
		return nil, false, err
	}
	if err = workflowWrite(st, a, w); err != nil {
		return nil, false, err
	}
	if w.Published || w.DraftRevision != in.ExpectedRevision {
		return nil, false, failure(409, "workflow_conflict", "Publish the exact current saved draft revision")
	}
	if err = ValidateWorkflow(w, true); err != nil {
		return nil, false, err
	}
	w.Published, w.DraftRevision = true, 0
	if _, exists := st.Workflows[workflowKey(w)]; exists {
		return nil, false, failure(409, "workflow_conflict", "Published versions are immutable")
	}
	st.Workflows[workflowKey(w)] = w
	delete(st.Workflows, w.ID+"@draft")
	remember(st, key, fingerprint, w)
	return w, true, nil
}
