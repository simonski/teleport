package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"teleport/internal/localruntime"
	"teleport/internal/model"
)

func localActor(st *model.State, token string) (model.Account, bool) {
	if token == "" {
		return model.Account{}, false
	}
	hash := digest(token)
	for _, row := range st.LocalSessions {
		if row.CredentialHash == hash && row.State == "running" && !row.StopRequested && time.Now().Before(row.CredentialExpires) {
			owner, ok := st.Accounts[row.Creator]
			binding := st.Projects[row.ProjectID].Repository
			if !ok || owner.Suspended || len(st.Accounts) != 1 || require(st, owner, row.ProjectID, "Contributor") != nil || binding == nil || binding.Version != row.BindingVersion {
				return model.Account{}, false
			}
			if row.TicketID != 0 {
				t := st.Tickets[row.TicketID]
				if t.Execution != "local:"+row.ID || t.StopRequested || t.Archived || t.Deleted || t.Status == "Cancelled" {
					return model.Account{}, false
				}
			}
			return model.Account{ID: "agent:" + row.ID, Username: "agent:" + row.ID, ServiceSessionID: row.ID, ServiceProjectID: row.ProjectID}, true
		}
	}
	return model.Account{}, false
}
func nativeExecutable(ctx context.Context) (string, bool) {
	if !localruntime.Supported() {
		return "", false
	}
	path, err := exec.LookPath("codex")
	if err != nil {
		return "", false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, path, "--version").Output()
	return path, err == nil && strings.TrimSpace(string(output)) == "codex-cli 0.166.0"
}

type NativeRequestsInput struct {
	ID string `json:"id"`
}
type NativeRequestPage struct {
	Gap   bool                         `json:"gap"`
	Items []localruntime.NativeRequest `json:"items"`
}
type NativeResponseInput struct {
	ID          string                               `json:"id"`
	RequestID   string                               `json:"requestId"`
	OperationID string                               `json:"operationId"`
	Answers     map[string]localruntime.NativeAnswer `json:"answers,omitempty"`
	Decision    string                               `json:"decision,omitempty"`
}

func (s *Service) nativeRequestCall(ctx context.Context, token, op string, raw json.RawMessage) (any, error) {
	var input NativeResponseInput
	if op == "getSessionRequests" {
		var in NativeRequestsInput
		if err := parse(raw, &in); err != nil {
			return nil, err
		}
		input.ID = in.ID
	} else if err := parse(raw, &input); err != nil {
		return nil, err
	}
	_, _, row, err := s.localSnapshot(ctx, token, input.ID)
	if err != nil {
		return nil, err
	}
	if s.localClient == nil {
		return nil, failure(503, "runtime_unavailable", "Native requests require the owned runtime")
	}
	if op == "getSessionRequests" {
		out, err := s.localClient.Call(ctx, daemonRequest(row, "nativeRequests"))
		if err != nil {
			return nil, failure(503, "native_requests_unavailable", "Native request history is unavailable; no replacement was started")
		}
		if _, _, _, err = s.localSnapshot(ctx, token, input.ID); err != nil {
			return nil, err
		}
		return NativeRequestPage{Items: append([]localruntime.NativeRequest{}, out.Requests...), Gap: out.RequestsGap}, nil
	}
	var receipt localruntime.NativeReceipt
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		a, err := actor(st, token)
		if err != nil {
			return false, err
		}
		current := st.LocalSessions[row.ID]
		if err = localAllowed(st, a, current); err != nil {
			return false, err
		}
		binding := st.Projects[current.ProjectID].Repository
		if len(st.Accounts) != 1 || current.State != "running" || current.StopRequested || binding == nil || binding.Version != current.BindingVersion {
			return false, failure(409, "native_reply_unavailable", "Reply only to the same live instance")
		}
		if current.TicketID != 0 {
			t := st.Tickets[current.TicketID]
			if t.StopRequested || t.ResumeHold || t.Execution != "local:"+current.ID {
				return false, failure(409, "native_reply_unavailable", "Ticket execution authority changed")
			}
		}
		request := daemonRequest(current, "nativeReply")
		request.NativeReply = &localruntime.NativeReply{RequestID: input.RequestID, OperationID: input.OperationID, Answers: input.Answers, Decision: input.Decision}
		out, err := s.localClient.Call(ctx, request)
		if err != nil {
			return false, failure(409, "native_reply_not_delivered", "Native request expired, changed, or could not be confirmed; inspect its current state before retrying")
		}
		if out.NativeReceipt == nil {
			return false, failure(503, "native_reply_unconfirmed", "Native response receipt is unavailable")
		}
		receipt = *out.NativeReceipt
		return false, nil
	})
	return receipt, err
}
func (s *Service) writeContextCredential(row model.LocalSession) (string, error) {
	if s.localEndpoint == "" {
		return "", failure(409, "local_endpoint_required", "Configure an explicit local server endpoint for managed context reads")
	}
	path := filepath.Join(s.localClient.Root, row.ID+".client.json")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return "", failure(409, "context_credential_conflict", "Managed context credential ownership changed")
		}
		data, err := os.ReadFile(path)
		var saved map[string]string
		if err != nil || len(data) > 4096 || json.Unmarshal(data, &saved) != nil || saved["kind"] != "teleport-local-context" || saved["sessionId"] != row.ID || saved["launchId"] != row.LaunchID || saved["server"] != s.localEndpoint || len(saved["token"]) != 64 {
			return "", failure(409, "context_credential_conflict", "Managed context credential basis changed")
		}
		hash := digest(saved["token"])
		if row.CredentialHash != "" && row.CredentialHash != hash {
			return "", failure(409, "context_credential_conflict", "Managed context credential changed")
		}
		return hash, nil
	}
	token := randomID()
	data, _ := json.Marshal(map[string]string{"server": s.localEndpoint, "token": token, "kind": "teleport-local-context", "sessionId": row.ID, "launchId": row.LaunchID})
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	return digest(token), nil
}
