package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"teleport/internal/model"
)

type BacklogOrderInput struct {
	ProjectID int64 `json:"projectId,omitempty"`
}
type BacklogMove struct {
	ID                    int64 `json:"id"`
	ExpectedVersion       int64 `json:"expectedVersion"`
	ParentID              int64 `json:"parentId"`
	ExpectedParentVersion int64 `json:"expectedParentVersion"`
}
type SaveBacklogInput struct {
	ProjectID       int64        `json:"projectId,omitempty"`
	ExpectedVersion int64        `json:"expectedVersion"`
	Keys            []string     `json:"keys"`
	Move            *BacklogMove `json:"move,omitempty"`
}

func backlogKeyAllowed(st *model.State, a model.Account, key string, project int64) bool {
	if strings.HasPrefix(key, "ticket:") {
		id, err := strconv.ParseInt(strings.TrimPrefix(key, "ticket:"), 10, 64)
		if err != nil || id <= 0 || key != fmt.Sprintf("ticket:%d", id) {
			return false
		}
		t, err := ticketFor(st, a, id, "Viewer")
		return err == nil && !t.Archived && !t.Deleted && (project == 0 || project == t.ProjectID)
	}
	if strings.HasPrefix(key, "idea:") {
		idea, err := ideaFor(st, a, key, "Viewer")
		return err == nil && !idea.Archived && !idea.Deleted && (project == 0 || project == idea.ProjectID)
	}
	return false
}

func backlogOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	var in SaveBacklogInput
	if operation == "getBacklogOrder" {
		var read BacklogOrderInput
		if err := parse(raw, &read); err != nil {
			return nil, false, err
		}
		in.ProjectID = read.ProjectID
	} else if err := parse(raw, &in); err != nil {
		return nil, false, err
	}
	if in.ProjectID != 0 {
		if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
			return nil, false, err
		}
	}
	scope := fmt.Sprintf("%s:%d", a.ID, in.ProjectID)
	order := st.BacklogOrders[scope]
	if operation == "getBacklogOrder" {
		visible := []string{}
		for _, key := range order.Keys {
			if backlogKeyAllowed(st, a, key, in.ProjectID) {
				visible = append(visible, key)
			}
		}
		return model.BacklogOrder{Version: order.Version, Keys: visible}, false, nil
	}
	if order.Keys == nil {
		order.Keys = []string{}
	}
	if in.Keys == nil {
		in.Keys = []string{}
	}
	if order.Version != in.ExpectedVersion {
		return nil, false, failure(409, "backlog_conflict", "Backlog order changed; refresh before retrying")
	}
	if len(in.Keys) > 2000 {
		return nil, false, failure(422, "backlog_too_large", "Order at most 2000 visible backlog items")
	}
	seen := map[string]bool{}
	for _, key := range in.Keys {
		if seen[key] || !backlogKeyAllowed(st, a, key, in.ProjectID) {
			return nil, false, failure(422, "invalid_backlog_order", "Use unique currently visible items in this scope")
		}
		seen[key] = true
	}
	moved := false
	if in.Move != nil {
		move := in.Move
		t, err := ticketFor(st, a, move.ID, "Contributor")
		if err != nil {
			return nil, false, err
		}
		if in.ProjectID != 0 && t.ProjectID != in.ProjectID || !seen[fmt.Sprintf("ticket:%d", t.ID)] {
			return nil, false, failure(422, "invalid_backlog_order", "Moved ticket must belong to this visible backlog")
		}
		if err = expected(t, move.ExpectedVersion); err != nil {
			return nil, false, err
		}
		if t.ParentID != move.ParentID {
			for _, id := range subtree(st, t.ID) {
				child := st.Tickets[id]
				if child.Status != "Todo" || busy(child) || child.Delivery != nil || child.Protected || child.Archived || child.Deleted {
					return nil, false, failure(409, "backlog_locked", "Only unlocked Todo work without retained implementation can change parent")
				}
			}
			if move.ParentID != 0 {
				parent, readErr := ticketFor(st, a, move.ParentID, "Viewer")
				if readErr != nil {
					return nil, false, readErr
				}
				if err = expected(parent, move.ExpectedParentVersion); err != nil {
					return nil, false, err
				}
			}
			body, _ := json.Marshal(MoveInput{ID: t.ID, ExpectedVersion: t.Version, ParentID: move.ParentID})
			if _, _, err = controlOperations(st, a, "moveTicket", body); err != nil {
				return nil, false, err
			}
			moved = true
		}
	}
	if !moved && reflect.DeepEqual(order.Keys, in.Keys) {
		return order, false, nil
	}
	order.Version++
	order.Keys = append([]string{}, in.Keys...)
	if st.BacklogOrders == nil {
		st.BacklogOrders = map[string]model.BacklogOrder{}
	}
	st.BacklogOrders[scope] = order
	return order, true, nil
}
