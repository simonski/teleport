package model

type Point3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}
type GraphLayout struct {
	Version   int64             `json:"version"`
	Positions map[string]Point3 `json:"positions"`
}
type BacklogOrder struct {
	Version int64    `json:"version"`
	Keys    []string `json:"keys"`
}
type RecentContext struct {
	View      string `json:"view"`
	ProjectID int64  `json:"projectId"`
	TicketID  int64  `json:"ticketId,omitempty"`
}
type Preferences struct {
	Version             int64           `json:"version"`
	Style               string          `json:"style"`
	Mode                string          `json:"mode"`
	DefaultView         string          `json:"defaultView"`
	DefaultProject      int64           `json:"defaultProject"`
	Recent              []RecentContext `json:"recent"`
	NavigationOrder     []string        `json:"navigationOrder"`
	NavigationPosition  string          `json:"navigationPosition"`
	NavigationCollapsed bool            `json:"navigationCollapsed"`
}
