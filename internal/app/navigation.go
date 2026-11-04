package app

func canonicalNavigationOrder() []string {
	return []string{"Home", "Backlog", "Board", "Todos", "Calendar", "Graph", "Workflows", "Agents", "Evolution", "Cost"}
}

func canonicalView(name string) string {
	switch name {
	case "Ideas", "Goals", "Epics", "Tickets":
		return "Backlog"
	case "Projects":
		return "Home"
	}
	return name
}

func normalizeNavigationOrder(saved []string) []string {
	order, seen := []string{}, map[string]bool{}
	canonical := canonicalNavigationOrder()
	for _, name := range append(append([]string{}, saved...), canonical...) {
		name = canonicalView(name)
		if contains(canonical, name) && !seen[name] {
			order = append(order, name)
			seen[name] = true
		}
	}
	return order
}

func validNavigationOrder(order []string) bool {
	allowed := canonicalNavigationOrder()
	if len(order) == 9 {
		allowed = []string{"Home", "Backlog", "Board", "Todos", "Calendar", "Graph", "Workflows", "Evolution", "Cost"}
	}
	if len(order) == 12 || len(order) == 13 {
		allowed = []string{"Home", "Ideas", "Goals", "Epics", "Board", "Todos", "Calendar", "Graph", "Workflows", "Evolution", "Cost", "Projects"}
		if len(order) == 13 {
			allowed = append(allowed, "Tickets")
		}
	}
	if len(order) != len(allowed) {
		return false
	}
	seen := map[string]bool{}
	for _, name := range order {
		if seen[name] || !contains(allowed, name) {
			return false
		}
		seen[name] = true
	}
	return true
}
