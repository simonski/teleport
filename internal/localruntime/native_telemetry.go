package localruntime

import (
	"encoding/json"
	"unicode/utf8"
)

const nativeTelemetryLimit = 65536

func validNativeID(id string) bool { return id != "" && len(id) <= 256 && utf8.ValidString(id) }

func compactAgentItem(raw json.RawMessage) (json.RawMessage, bool) {
	var item struct {
		Type      string  `json:"type"`
		Phase     *string `json:"phase,omitempty"`
		Text      string  `json:"text"`
		Truncated bool    `json:"truncated,omitempty"`
	}
	if json.Unmarshal(raw, &item) != nil || item.Type != "agentMessage" || item.Phase != nil && *item.Phase != "final_answer" {
		return nil, false
	}
	if len(item.Text) > NativeTurnTextLimit {
		item.Truncated = true
	}
	data, _ := json.Marshal(item)
	if len(data) > 48<<10 {
		item.Truncated = true
	}
	if item.Truncated {
		item.Text = ""
		data, _ = json.Marshal(item)
	}
	return data, item.Truncated
}

// Telemetry is independent of the unchanged wire stream forwarded to the terminal.
func compactNativeTelemetry(data []byte) ([]byte, bool) {
	var message nativeMessage
	if json.Unmarshal(data, &message) != nil {
		return nil, false
	}
	if len(message.ID) > 0 {
		return data, len(data) <= nativeTelemetryLimit
	}
	var params any
	switch message.Method {
	case "thread/started":
		var in struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if json.Unmarshal(message.Params, &in) != nil || !validNativeID(in.Thread.ID) {
			return nil, false
		}
		params = in
	case "turn/started", "turn/completed":
		var in struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID     string            `json:"id"`
				Status string            `json:"status,omitempty"`
				Items  []json.RawMessage `json:"items,omitempty"`
			} `json:"turn"`
		}
		if json.Unmarshal(message.Params, &in) != nil || !validNativeID(in.ThreadID) || !validNativeID(in.Turn.ID) {
			return nil, false
		}
		var last json.RawMessage
		truncated := false
		if message.Method == "turn/completed" {
			for _, raw := range in.Turn.Items {
				if item, tooLarge := compactAgentItem(raw); item != nil {
					last, truncated = item, truncated || tooLarge
				}
			}
		}
		in.Turn.Items = nil
		if truncated {
			last = json.RawMessage(`{"type":"agentMessage","phase":"final_answer","text":"","truncated":true}`)
		}
		if last != nil {
			in.Turn.Items = []json.RawMessage{last}
		}
		params = in
	case "item/completed":
		var in struct {
			ThreadID string          `json:"threadId"`
			TurnID   string          `json:"turnId"`
			Item     json.RawMessage `json:"item"`
		}
		if json.Unmarshal(message.Params, &in) != nil || !validNativeID(in.ThreadID) || !validNativeID(in.TurnID) {
			return nil, false
		}
		item, _ := compactAgentItem(in.Item)
		if item == nil {
			return nil, true
		}
		in.Item = item
		params = in
	default:
		return data, len(data) <= nativeTelemetryLimit
	}
	encoded, _ := json.Marshal(map[string]any{"method": message.Method, "params": params})
	return encoded, len(encoded) <= nativeTelemetryLimit
}
