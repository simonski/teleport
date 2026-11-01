package app

func canonicalNavigationOrder() []string {
	return []string{"Home", "Ideas", "Goals", "Epics", "Board", "Todos", "Calendar", "Graph", "Workflows", "Evolution", "Cost", "Projects"}
}

func normalizeNavigationOrder(saved []string) []string {
	order, seen := []string{}, map[string]bool{}
	canonical := canonicalNavigationOrder()
	for _, name := range append(append([]string{}, saved...), canonical...) {
		if contains(canonical, name) && !seen[name] {
			order = append(order, name)
			seen[name] = true
		}
	}
	return order
}

func validNavigationOrder(order []string) bool {
	if len(order) != len(canonicalNavigationOrder()) {
		return false
	}
	seen := map[string]bool{}
	for _, name := range order {
		if seen[name] || !contains(canonicalNavigationOrder(), name) {
			return false
		}
		seen[name] = true
	}
	return true
}
