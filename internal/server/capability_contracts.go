package server

import "teleport/internal/app"

func init() {
	Operations = append(Operations, Operation{"getEffectivePermission", "ACT-1113", "Inspect unsupported native execution without authorizing dispatch"})
	contracts["getEffectivePermission"] = contract{app.PermissionInspectionInput{}, app.PermissionInspection{}}
}
