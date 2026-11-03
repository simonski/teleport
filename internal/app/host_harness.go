package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"teleport/internal/hostharness"
	"teleport/internal/model"
)

type HostHarnessList struct {
	Items []hostharness.Process `json:"items"`
	Scope string                `json:"scope"`
}
type HostHarnessStopInput struct {
	PID      int    `json:"pid"`
	Identity string `json:"identity"`
	Confirm  bool   `json:"confirm"`
	Force    bool   `json:"force,omitempty"`
}
type HostHarnessStopResult struct {
	PID   int    `json:"pid"`
	State string `json:"state"`
}

func hostHarnessActor(st *model.State, token string) (model.Account, error) {
	a, err := actor(st, token)
	if err != nil {
		return a, err
	}
	if !a.Administrator || a.ServiceSessionID != "" || a.ServiceRunID != "" {
		return a, failure(403, "forbidden", "Only a human installation administrator can inspect host processes")
	}
	if len(st.Accounts) != 1 {
		return a, failure(409, "isolation_required", "Host process inspection requires single-user local mode")
	}
	return a, nil
}
func (s *Service) hostHarnessCall(ctx context.Context, token, operation string, raw json.RawMessage) (any, error) {
	if operation == "listHostHarnesses" {
		if err := parse(raw, &struct{}{}); err != nil {
			return nil, err
		}
		st, err := s.db.Read(ctx)
		if err != nil {
			return nil, err
		}
		if _, err = hostHarnessActor(st, token); err != nil {
			return nil, err
		}
		rows, err := hostharness.List(ctx)
		if err != nil {
			return nil, failure(503, "process_inventory_unavailable", "Cannot inspect local harness processes")
		}
		st, err = s.db.Read(ctx)
		if err != nil {
			return nil, err
		}
		if _, err = hostHarnessActor(st, token); err != nil {
			return nil, err
		}
		return HostHarnessList{Items: rows, Scope: "same operating-system user, recognized harness executables and supported interpreter entrypoints"}, nil
	}
	var in HostHarnessStopInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	if !in.Confirm {
		return nil, failure(422, "confirmation_required", "Confirm stopping this exact process")
	}
	var actorID string
	err := s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := hostHarnessActor(st, token)
		if err != nil {
			return false, err
		}
		actorID = a.ID
		if err = hostharness.Stop(ctx, in.PID, in.Identity, in.Force); err != nil {
			return false, failure(409, "process_changed", err.Error())
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	slog.Info("external harness stop requested", "actor", actorID, "pid", in.PID, "force", in.Force)
	return HostHarnessStopResult{PID: in.PID, State: "signal_sent"}, nil
}
