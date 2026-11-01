package localruntime

import (
	"encoding/json"
	"time"
)

const NativeTurnTextLimit = 32 << 10

type NativeTurn struct {
	ThreadID  string    `json:"threadId"`
	TurnID    string    `json:"turnId"`
	State     string    `json:"state"`
	Text      string    `json:"text"`
	ErrorCode string    `json:"errorCode,omitempty"`
	Truncated bool      `json:"truncated"`
	At        time.Time `json:"at"`
}

func (turn *NativeTurn) capture(item json.RawMessage) {
	var message struct {
		Type      string  `json:"type"`
		Text      string  `json:"text"`
		Phase     *string `json:"phase"`
		Truncated bool    `json:"truncated"`
	}
	if json.Unmarshal(item, &message) != nil || message.Type != "agentMessage" || message.Phase != nil && *message.Phase != "final_answer" {
		return
	}
	if message.Truncated || len(message.Text) > NativeTurnTextLimit {
		turn.Truncated = true
		turn.ErrorCode = "native_result_too_large"
		turn.Text = ""
		return
	}
	if !turn.Truncated {
		turn.Text = message.Text
	}
}
