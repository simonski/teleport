package localruntime

import "time"

type NativeFailure struct {
	Code           string    `json:"code"`
	At             time.Time `json:"at"`
	EngineExitCode *int      `json:"engineExitCode,omitempty"`
}

func validNativeFailure(code string) bool {
	switch code {
	case "native_protocol_invalid_json", "native_protocol_thread_changed", "native_protocol_event_thread_changed", "native_protocol_frame_too_large", "native_protocol_read_failed", "native_protocol_eof", "native_frontend_write_failed", "native_capture_unavailable":
		return true
	}
	return false
}
