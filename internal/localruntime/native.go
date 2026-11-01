package localruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type NativeOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}
type NativeQuestion struct {
	ID       string         `json:"id"`
	Header   string         `json:"header"`
	Question string         `json:"question"`
	IsSecret bool           `json:"isSecret"`
	IsOther  bool           `json:"isOther"`
	Options  []NativeOption `json:"options,omitempty"`
}
type NativeAnswer struct {
	Answers []string `json:"answers"`
}
type NativeRequest struct {
	ID         string                  `json:"id"`
	RPCID      json.RawMessage         `json:"rpcId"`
	ThreadID   string                  `json:"threadId"`
	TurnID     string                  `json:"turnId"`
	Method     string                  `json:"method"`
	State      string                  `json:"state"`
	Questions  []NativeQuestion        `json:"questions,omitempty"`
	Decisions  []string                `json:"decisions,omitempty"`
	Command    string                  `json:"command,omitempty"`
	Reason     string                  `json:"reason,omitempty"`
	Answers    map[string]NativeAnswer `json:"answers,omitempty"`
	Decision   string                  `json:"decision,omitempty"`
	Source     string                  `json:"source,omitempty"`
	AskedAt    time.Time               `json:"askedAt"`
	ResolvedAt *time.Time              `json:"resolvedAt,omitempty"`
}
type NativeReply struct {
	RequestID   string                  `json:"requestId"`
	OperationID string                  `json:"operationId"`
	Answers     map[string]NativeAnswer `json:"answers,omitempty"`
	Decision    string                  `json:"decision,omitempty"`
}
type NativeReceipt struct {
	OperationID string `json:"operationId"`
	State       string `json:"state"`
}
type NativeEvent struct {
	RequestID string          `json:"requestId,omitempty"`
	Server    bool            `json:"server"`
	Message   json.RawMessage `json:"message"`
}
type nativeMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

func nativeID(raw json.RawMessage) string {
	var text string
	if len(raw) == 0 || len(raw) > 4096 {
		return ""
	}
	if raw[0] == '"' {
		if json.Unmarshal(raw, &text) != nil || text == "" {
			return ""
		}
	} else if _, err := strconv.ParseInt(string(raw), 10, 64); err != nil {
		return ""
	}
	hash := sha256.Sum256(raw)
	return "N" + hex.EncodeToString(hash[:12])
}
func decodeNativeQuestion(message nativeMessage) (NativeRequest, error) {
	var params struct {
		ThreadID           string            `json:"threadId"`
		TurnID             string            `json:"turnId"`
		Questions          []NativeQuestion  `json:"questions"`
		AvailableDecisions []json.RawMessage `json:"availableDecisions"`
		Command            *string           `json:"command"`
		Reason             *string           `json:"reason"`
	}
	if len(message.Params) > 65536 || json.Unmarshal(message.Params, &params) != nil || params.ThreadID == "" || nativeID(message.ID) == "" {
		return NativeRequest{}, errors.New("invalid_native_request")
	}
	scopedID, _ := json.Marshal(params.ThreadID + "\x00" + params.TurnID + "\x00" + string(message.ID))
	request := NativeRequest{ID: nativeID(scopedID), RPCID: message.ID, ThreadID: params.ThreadID, TurnID: params.TurnID, Method: message.Method, State: "pending", AskedAt: time.Now().UTC()}
	if params.Command != nil {
		request.Command = *params.Command
	}
	if params.Reason != nil {
		request.Reason = *params.Reason
	}
	if message.Method == "item/tool/requestUserInput" {
		if len(params.Questions) < 1 || len(params.Questions) > 10 {
			return request, errors.New("unsupported_native_questions")
		}
		seen := map[string]bool{}
		for _, q := range params.Questions {
			if q.ID == "" || seen[q.ID] || len(q.ID) > 200 || len(q.Question) > 20000 || len(q.Options) > 20 {
				return request, errors.New("unsupported_native_questions")
			}
			seen[q.ID] = true
		}
		request.Questions = params.Questions
	} else if message.Method == "item/commandExecution/requestApproval" || message.Method == "item/fileChange/requestApproval" {
		request.Decisions = []string{}
		if len(params.AvailableDecisions) == 0 {
			request.Decisions = []string{"accept", "acceptForSession", "decline", "cancel"}
		} else {
			for _, raw := range params.AvailableDecisions {
				var decision string
				if json.Unmarshal(raw, &decision) == nil && nativeDecision(decision) {
					request.Decisions = append(request.Decisions, decision)
				}
			}
		}
	} else {
		request.Reason = "Respond to this native request in the terminal"
	}
	return request, nil
}
func nativeDecision(value string) bool {
	return value == "accept" || value == "acceptForSession" || value == "decline" || value == "cancel"
}
func NativeReplyResult(request NativeRequest, reply NativeReply) (json.RawMessage, error) {
	if request.State != "pending" || request.ID != reply.RequestID || reply.OperationID == "" || len(reply.OperationID) > 100 {
		return nil, errors.New("native_request_not_pending")
	}
	if request.Method == "item/tool/requestUserInput" {
		if reply.Decision != "" || len(reply.Answers) != len(request.Questions) {
			return nil, errors.New("complete_every_question")
		}
		total := 0
		for _, q := range request.Questions {
			answer, ok := reply.Answers[q.ID]
			if !ok || len(answer.Answers) < 1 || len(answer.Answers) > 10 {
				return nil, errors.New("complete_every_question")
			}
			for _, text := range answer.Answers {
				total += len(text)
				if strings.TrimSpace(text) == "" || len(text) > 20000 || strings.ContainsRune(text, 0) {
					return nil, errors.New("invalid_native_answer")
				}
				if len(q.Options) > 0 && !q.IsOther {
					found := false
					for _, option := range q.Options {
						if text == option.Label {
							found = true
						}
					}
					if !found {
						return nil, errors.New("choose_an_offered_answer")
					}
				}
			}
		}
		if total > 65536 {
			return nil, errors.New("native_answers_too_large")
		}
		result, err := json.Marshal(map[string]any{"answers": reply.Answers})
		if len(result) > 65536 {
			return nil, errors.New("native_answers_too_large")
		}
		return result, err
	}
	if len(reply.Answers) != 0 {
		return nil, errors.New("unexpected_native_answers")
	}
	for _, offered := range request.Decisions {
		if offered == reply.Decision {
			return json.Marshal(map[string]string{"decision": reply.Decision})
		}
	}
	return nil, errors.New("choose_an_offered_decision")
}
func retainNativeReply(request *NativeRequest, result json.RawMessage, source string) {
	var values struct {
		Answers  map[string]NativeAnswer `json:"answers"`
		Decision string                  `json:"decision"`
	}
	if json.Unmarshal(result, &values) != nil {
		return
	}
	for _, q := range request.Questions {
		if q.IsSecret {
			if _, ok := values.Answers[q.ID]; ok {
				values.Answers[q.ID] = NativeAnswer{Answers: []string{"[redacted]"}}
			}
		}
	}
	request.Answers, request.Decision, request.Source = values.Answers, values.Decision, source
}
