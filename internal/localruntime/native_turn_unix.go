//go:build darwin || linux

package localruntime

import "encoding/json"

func (d *daemon) trimNativeTurns() {
	for {
		total := 0
		var oldest *NativeTurn
		for _, e := range d.entries {
			if e.NativeTurn == nil {
				continue
			}
			data, _ := json.Marshal(e.NativeTurn)
			total += len(data)
			if (e.State == "exited" || e.State == "failed") && e.NativeTurn.Text != "" && (oldest == nil || e.NativeTurn.At.Before(oldest.At)) {
				oldest = e.NativeTurn
			}
		}
		if total <= 8<<20 || oldest == nil {
			return
		}
		oldest.Text = ""
		oldest.Truncated = true
		oldest.ErrorCode = "native_turn_retention_gap"
	}
}
