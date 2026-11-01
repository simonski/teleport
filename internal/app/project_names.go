package app

import (
	"strconv"
	"strings"

	"teleport/internal/model"
)

type ProjectAvailabilityInput struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type ProjectAvailability struct {
	NameAvailable bool   `json:"nameAvailable"`
	KeyAvailable  bool   `json:"keyAvailable"`
	SuggestedKey  string `json:"suggestedKey"`
}

func sameProjectName(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}

func projectAvailability(st *model.State, in ProjectAvailabilityInput) ProjectAvailability {
	r := ProjectAvailability{NameAvailable: strings.TrimSpace(in.Name) != "", KeyAvailable: in.Key != ""}
	keys := map[string]bool{}
	for _, p := range st.Projects {
		keys[p.Key] = true
		if sameProjectName(p.Name, in.Name) {
			r.NameAvailable = false
		}
		if p.Key == in.Key {
			r.KeyAvailable = false
		}
	}
	var letters strings.Builder
	for _, c := range strings.ToLower(in.Name) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			letters.WriteRune(c)
		}
		if letters.Len() == 4 {
			break
		}
	}
	base := letters.String()
	if base == "" {
		base = "proj"
	}
	r.SuggestedKey = base
	if keys[base] {
		if len(base) > 3 {
			base = base[:3]
		}
		for n := 1; ; n++ {
			r.SuggestedKey = base + strconv.Itoa(n)
			if !keys[r.SuggestedKey] {
				break
			}
		}
	}
	return r
}
