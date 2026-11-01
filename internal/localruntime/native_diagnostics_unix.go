//go:build darwin || linux

package localruntime

import "time"

func retainNativeFailure(e *entry, code string) {
	if !validNativeFailure(code) {
		return
	}
	if e.NativeFailure == nil {
		e.NativeFailure = &NativeFailure{Code: code, At: time.Now().UTC()}
	}
	if e.NativeTurn != nil && e.NativeTurn.State == "inProgress" {
		e.NativeTurn.State, e.NativeTurn.ErrorCode, e.NativeTurn.Text = "unknown", code, ""
	}
}
