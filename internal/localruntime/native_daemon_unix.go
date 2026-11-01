//go:build darwin || linux

package localruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"
)

type nativeReceiptRecord struct {
	NativeReceipt
	Fingerprint string `json:"fingerprint"`
}

func (d *daemon) nativeEvent(e *entry, event *NativeEvent) Response {
	if !e.Native || event == nil || len(event.Message) > 65536 || e.parentReaped || e.State != "running" && e.State != "stopping" {
		return Response{Error: "native_event_unavailable"}
	}
	var message nativeMessage
	if json.Unmarshal(event.Message, &message) != nil {
		return Response{Error: "invalid_native_event"}
	}
	if len(message.ID) > 0 && !d.nativeHistoryRoom(128<<10) {
		e.Attention = "Native request history capacity reached"
		_ = d.persist()
		return Response{Error: "native_history_capacity"}
	}
	if !event.Server {
		for i := range e.NativeRequests {
			request := &e.NativeRequests[i]
			if request.ID == event.RequestID && bytes.Equal(request.RPCID, message.ID) {
				retainNativeReply(request, message.Result, "native client")
				if request.State == "pending" {
					request.State = "responding"
				}
				break
			}
		}
	} else if len(message.ID) > 0 && message.Method != "" {
		request, err := decodeNativeQuestion(message)
		if err != nil {
			e.Attention = "Native input required"
			return Response{Error: "unsupported_native_request"}
		}
		if e.NativeThreadID != "" && request.ThreadID != e.NativeThreadID {
			return Response{Error: "native_thread_identity_changed"}
		}
		for _, existing := range e.NativeRequests {
			if existing.ID == request.ID {
				return Response{Generation: d.generation}
			}
		}
		for i := range e.NativeRequests {
			old := &e.NativeRequests[i]
			if bytes.Equal(old.RPCID, request.RPCID) && (old.State == "pending" || old.State == "responding") {
				old.State = "superseded"
			}
		}
		// Keep pending requests intact; prune only resolved history to bound
		// private manifests and API responses independently of turn count.
		for len(e.NativeRequests) > 0 {
			data, _ := json.Marshal(e.NativeRequests)
			if len(data) < 256<<10 && len(e.NativeRequests) < 128 {
				break
			}
			index := -1
			for i, r := range e.NativeRequests {
				if r.State != "pending" && r.State != "responding" {
					index = i
					break
				}
			}
			if index < 0 {
				break
			}
			e.NativeRequests = append(e.NativeRequests[:index], e.NativeRequests[index+1:]...)
			e.NativeHistoryGap = true
		}
		data, _ := json.Marshal(e.NativeRequests)
		if len(e.NativeRequests) >= 128 || len(data) >= 256<<10 {
			e.Attention = "Native request history limit reached"
			_ = d.persist()
			return Response{Error: "native_request_limit"}
		}
		e.NativeRequests = append(e.NativeRequests, request)
	} else {
		switch message.Method {
		case "teleport/captureError":
			e.Attention = "Native event capture unavailable; use terminal"
		case "teleport/nativeExit":
			var params struct {
				Confirmed bool `json:"confirmed"`
			}
			if json.Unmarshal(message.Params, &params) == nil {
				e.NativeReleased = params.Confirmed
			}
		case "thread/started":
			var params struct {
				Thread struct {
					ID string `json:"id"`
				} `json:"thread"`
			}
			if json.Unmarshal(message.Params, &params) == nil {
				if e.NativeThreadID != "" && e.NativeThreadID != params.Thread.ID {
					return Response{Error: "native_thread_identity_changed"}
				}
				e.NativeThreadID = params.Thread.ID
				e.Attention = ""
			}
		case "serverRequest/resolved":
			var params struct {
				ThreadID  string          `json:"threadId"`
				RequestID json.RawMessage `json:"requestId"`
			}
			if json.Unmarshal(message.Params, &params) == nil {
				for i := range e.NativeRequests {
					request := &e.NativeRequests[i]
					if request.ID == event.RequestID && request.ThreadID == params.ThreadID && bytes.Equal(request.RPCID, params.RequestID) && (request.State == "pending" || request.State == "responding") {
						now := time.Now().UTC()
						request.ResolvedAt = &now
						request.State = "resolved"
					}
				}
				e.Attention = ""
			}
		case "turn/started":
			e.Attention = ""
		case "turn/completed":
			e.Attention = "Response ready"
		case "thread/status/changed":
			var params struct {
				ThreadID string `json:"threadId"`
				Status   struct {
					Type  string   `json:"type"`
					Flags []string `json:"activeFlags"`
				} `json:"status"`
			}
			if json.Unmarshal(message.Params, &params) == nil && params.ThreadID == e.NativeThreadID {
				for _, flag := range params.Status.Flags {
					if flag == "waitingOnApproval" {
						e.Attention = "Native permission needs a decision"
					}
					if flag == "waitingOnUserInput" {
						e.Attention = "Question needs an answer"
					}
				}
				if params.Status.Type == "systemError" {
					e.Attention = "Native harness error"
				}
			}
		}
	}
	for _, request := range e.NativeRequests {
		if request.State == "pending" || request.State == "responding" {
			if request.Method == "item/tool/requestUserInput" {
				e.Attention = "Question needs an answer"
			} else {
				e.Attention = "Native input needs attention"
			}
			break
		}
	}
	if d.persist() != nil {
		return Response{Error: "native_event_persistence_failed"}
	}
	return Response{Generation: d.generation}
}
func (d *daemon) stopNative(id, key string) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", nativeSocket(d.root, id))
	}}
	defer transport.CloseIdleConnections()
	request, _ := http.NewRequest("POST", "http://native/stop", nil)
	request.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err == nil {
		response.Body.Close()
	}
	if err != nil || response.StatusCode != 200 {
		d.mu.Lock()
		if e := d.entries[id]; e != nil && e.State == "stopping" {
			e.ErrorCode = "native_stop_unconfirmed"
			_ = d.persist()
		}
		d.mu.Unlock()
	}
}
func (d *daemon) nativeReply(e *entry, reply *NativeReply) Response {
	if !e.Native || reply == nil || e.State != "running" || e.parentReaped {
		return Response{Error: "native_reply_unavailable"}
	}
	fingerprint, err := New(d.root, "").Fingerprint(reply)
	if err != nil {
		return Response{Error: "native_reply_identity_unavailable"}
	}
	if prior, ok := e.NativeReceipts[reply.OperationID]; ok {
		if prior.Fingerprint != fingerprint {
			return Response{Error: "native_reply_identity_conflict"}
		}
		receipt := prior.NativeReceipt
		return Response{NativeReceipt: &receipt}
	}
	if !d.nativeHistoryRoom(128 << 10) {
		return Response{Error: "native_history_capacity"}
	}
	var pending *NativeRequest
	for i := range e.NativeRequests {
		if e.NativeRequests[i].ID == reply.RequestID {
			pending = &e.NativeRequests[i]
			break
		}
	}
	if pending == nil {
		return Response{Error: "native_request_missing"}
	}
	result, err := NativeReplyResult(*pending, *reply)
	if err != nil {
		return Response{Error: err.Error()}
	}
	if len(e.NativeReceipts) >= 64 {
		return Response{Error: "native_reply_limit"}
	}
	record := nativeReceiptRecord{NativeReceipt: NativeReceipt{OperationID: reply.OperationID, State: "uncertain"}, Fingerprint: fingerprint}
	e.NativeReceipts[reply.OperationID] = record
	if d.persist() != nil {
		return Response{Error: "native_reply_intent_persistence_failed"}
	}
	dispatch := nativeDispatch{RequestID: pending.ID, OperationID: reply.OperationID, RPCID: pending.RPCID, Result: result}
	body, _ := json.Marshal(dispatch)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", nativeSocket(d.root, e.ID))
	}}
	defer transport.CloseIdleConnections()
	request, _ := http.NewRequest("POST", "http://native/respond", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+e.NativeKey)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err == nil {
		defer response.Body.Close()
		var observed NativeReceipt
		if response.StatusCode == 200 && json.NewDecoder(response.Body).Decode(&observed) == nil && observed.OperationID == reply.OperationID {
			record.NativeReceipt = observed
		}
	}
	if record.State == "delivered" {
		retainNativeReply(pending, result, "ticket/teleport dialog")
		pending.State = "responding"
	}
	e.NativeReceipts[reply.OperationID] = record
	if d.persist() != nil {
		return Response{Error: "native_reply_ack_persistence_failed"}
	}
	out := record.NativeReceipt
	return Response{NativeReceipt: &out}
}

// Bound aggregate retained content below the manifest recovery limit. Only
// settled history may expire; pending overflow refuses capture and GUI input.
func (d *daemon) nativeHistoryRoom(extra int) bool {
	for {
		total := extra
		for _, e := range d.entries {
			data, _ := json.Marshal(e.NativeRequests)
			total += len(data)
		}
		if total <= 16<<20 {
			return true
		}
		removed := false
		for _, e := range d.entries {
			for i, r := range e.NativeRequests {
				if r.State != "pending" && r.State != "responding" {
					e.NativeRequests = append(e.NativeRequests[:i], e.NativeRequests[i+1:]...)
					e.NativeHistoryGap = true
					removed = true
					break
				}
			}
			if removed {
				break
			}
		}
		if !removed {
			return false
		}
	}
}
