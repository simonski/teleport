package model

import (
	"encoding/json"
	"time"
)

type ScriptCall struct {
	ID                string          `json:"id"`
	Operation         string          `json:"operation"`
	Arguments         json.RawMessage `json:"arguments"`
	Fingerprint       string          `json:"fingerprint"`
	Basis             string          `json:"basis,omitempty"`
	State             string          `json:"state"`
	Approved          bool            `json:"approved"`
	ConfirmationActor string          `json:"confirmationActor,omitempty"`
	ConfirmedAt       *time.Time      `json:"confirmedAt,omitempty"`
	Decision          string          `json:"decision,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
	ErrorCode         string          `json:"errorCode,omitempty"`
	ErrorMessage      string          `json:"errorMessage,omitempty"`
	At                time.Time       `json:"at"`
}
type ScriptRun struct {
	StopActor     string       `json:"stopActor,omitempty"`
	StopAt        *time.Time   `json:"stopAt,omitempty"`
	ID            string       `json:"id"`
	Number        int64        `json:"number"`
	Creator       string       `json:"creator"`
	RequestID     string       `json:"requestId"`
	Source        string       `json:"source"`
	Mode          string       `json:"mode"`
	Fingerprint   string       `json:"fingerprint"`
	TokenHash     string       `json:"tokenHash"`
	LaunchID      string       `json:"launchId"`
	Generation    string       `json:"generation"`
	State         string       `json:"state"`
	RuntimeState  string       `json:"runtimeState"`
	StopRequested bool         `json:"stopRequested"`
	ErrorCode     string       `json:"errorCode,omitempty"`
	Projects      []int64      `json:"projects"`
	Calls         []ScriptCall `json:"calls"`
	Output        string       `json:"output"`
	Cursor        int64        `json:"cursor"`
	CreatedAt     time.Time    `json:"createdAt"`
	ExpiresAt     time.Time    `json:"expiresAt"`
	FinishedAt    *time.Time   `json:"finishedAt,omitempty"`
}

// WithdrawSuspendedPrivateExecutions runs in the same server transaction as
// account changes. Reactivation cannot undo a committed execution withdrawal.
func (st *State) WithdrawSuspendedPrivateExecutions() {
	for id, row := range st.ScriptRuns {
		a, ok := st.Accounts[row.Creator]
		if ok && !a.Suspended {
			continue
		}
		if row.State == "completed" || row.State == "failed" || row.State == "stopped" || row.State == "interrupted" {
			continue
		}
		row.StopRequested = true
		if row.StopActor == "" {
			now := time.Now().UTC()
			row.StopActor = "account-state"
			row.StopAt = &now
		}
		if row.State != "unknown" {
			row.State = "stopping"
		}
		row.ErrorCode = "script_account_withdrawn"
		for i := range row.Calls {
			if row.Calls[i].State == "waiting" || row.Calls[i].State == "approved" {
				row.Calls[i].State = "cancelled"
			}
		}
		st.ScriptRuns[id] = row
	}
	for id, row := range st.LocalSessions {
		if row.TicketID != 0 || row.State == "exited" || row.State == "failed" {
			continue
		}
		a, ok := st.Accounts[row.Creator]
		if ok && !a.Suspended {
			continue
		}
		row.StopRequested = true
		st.LocalSessions[id] = row
	}
}
