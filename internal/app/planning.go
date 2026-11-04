package app

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"teleport/internal/model"
)

type SearchHit struct {
	Ticket  model.Ticket `json:"ticket"`
	Score   int          `json:"score"`
	Matches []string     `json:"matches"`
}
type searchCursor struct {
	Query   string    `json:"query"`
	Score   int       `json:"score"`
	Updated time.Time `json:"updated"`
	ID      int64     `json:"id"`
}

func folded(s string) string { return cases.Fold().String(s) }
func textMatches(text, query string) bool {
	haystack := folded(text)
	for _, token := range strings.Fields(folded(query)) {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}
func searchRank(t model.Ticket, query string) (int, []string) {
	original := strings.Join(strings.Fields(query), " ")
	query = folded(original)
	title := folded(t.Title)
	id := strconv.FormatInt(t.ID, 10)
	if query == id || query == "#"+id {
		return 10000, []string{"id"}
	}
	if !textMatches(t.Title+" "+t.Description+" "+strings.Join(t.Labels, " ")+" "+t.Kind+" "+t.Priority+" "+id, query) {
		return 0, nil
	}
	base := 1000
	switch {
	case title == query:
		base = 9000
	case strings.HasPrefix(title, query):
		base = 7000
	case strings.Contains(title, query):
		base = 5000 + max(0, 200-strings.Index(title, query))
	case textMatches(title, query):
		base = 3000
	}
	if strings.Contains(t.Title, original) {
		base += 30
	}
	fields := []string{}
	for _, field := range []struct{ name, text string }{{"title", t.Title}, {"description", t.Description}, {"labels", strings.Join(t.Labels, " ")}, {"metadata", t.Kind + " " + t.Priority + " " + id}} {
		for _, token := range strings.Fields(query) {
			if strings.Contains(folded(field.text), token) {
				fields = append(fields, field.name)
				break
			}
		}
	}
	for _, token := range strings.Fields(query) {
		if strings.Contains(title, token) {
			base++
		}
	}
	return base, fields
}
func searchTickets(st *model.State, a model.Account, in ListInput) (Page[SearchHit], error) {
	result := Page[SearchHit]{Items: []SearchHit{}}
	in.Query = strings.Join(strings.Fields(in.Query), " ")
	if in.Query == "" || len(in.Query) > 500 || len(strings.Fields(in.Query)) > 20 {
		return result, failure(422, "invalid_query", "Use 1–20 search tokens within 500 bytes")
	}
	if in.ProjectID != 0 {
		if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
			return result, err
		}
	}
	_, limit, err := pageBounds("", in.Limit)
	if err != nil {
		return result, err
	}
	identity := in
	identity.Cursor = ""
	identity.Limit = 0
	encoded, _ := json.Marshal(struct {
		Actor string
		Input ListInput
	}{a.ID, identity})
	queryHash := digest(string(encoded))
	var after searchCursor
	if in.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		if err != nil || json.Unmarshal(data, &after) != nil || after.Query != queryHash || after.ID < 1 || after.Score < 1 || after.Score > 10020 || after.Updated.IsZero() {
			return result, failure(400, "invalid_cursor", "Refresh search after changing its query/scope")
		}
	}
	for _, t := range st.Tickets {
		if role(st, a, t.ProjectID) == "" || in.ProjectID != 0 && t.ProjectID != in.ProjectID || !in.IncludeHidden && (t.Archived || t.Deleted) || in.Kind != "" && t.Kind != in.Kind || in.Agent != nil && t.Agent != *in.Agent || in.Status != "" && t.Status != in.Status {
			continue
		}
		score, matches := searchRank(t, in.Query)
		if score == 0 {
			continue
		}
		if in.Cursor != "" && (score > after.Score || score == after.Score && (t.UpdatedAt.After(after.Updated) || t.UpdatedAt.Equal(after.Updated) && t.ID >= after.ID)) {
			continue
		}
		result.Items = append(result.Items, SearchHit{Ticket: t, Score: score, Matches: matches})
	}
	sort.Slice(result.Items, func(x, y int) bool {
		a, b := result.Items[x], result.Items[y]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if !a.Ticket.UpdatedAt.Equal(b.Ticket.UpdatedAt) {
			return a.Ticket.UpdatedAt.After(b.Ticket.UpdatedAt)
		}
		return a.Ticket.ID > b.Ticket.ID
	})
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		data, _ := json.Marshal(searchCursor{Query: queryHash, Score: last.Score, Updated: last.Ticket.UpdatedAt, ID: last.Ticket.ID})
		result.NextCursor = base64.RawURLEncoding.EncodeToString(data)
		result.Items = result.Items[:limit]
	}
	return result, nil
}

type CalendarInput struct {
	ProjectID int64  `json:"projectId,omitempty"`
	Date      string `json:"date"`
	Period    string `json:"period,omitempty"`
	DuePeriod string `json:"duePeriod,omitempty"`
	Query     string `json:"query,omitempty"`
	MaxRows   int    `json:"maxRows,omitempty"`
}
type CalendarRow struct {
	TicketID      int64  `json:"ticketId"`
	ProjectID     int64  `json:"projectId"`
	ParentID      int64  `json:"parentId"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	Progress      int    `json:"progress"`
	Start         string `json:"start"`
	End           string `json:"end"`
	ExplicitStart string `json:"explicitStart"`
	ExplicitETA   string `json:"explicitEta"`
	Due           string `json:"due"`
	Derived       bool   `json:"derived"`
	Milestone     bool   `json:"milestone"`
	Context       bool   `json:"context"`
}
type Calendar struct {
	Start       string        `json:"start"`
	End         string        `json:"end"`
	Rows        []CalendarRow `json:"rows"`
	Unscheduled int           `json:"unscheduled"`
}
type calendarBounds struct {
	start, end         string
	derived, milestone bool
}

func dateWindow(date time.Time, period string) (time.Time, time.Time, error) {
	start, end := date, date
	monday := date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
	switch period {
	case "today":
	case "week":
		start = monday
		end = start.AddDate(0, 0, 6)
	case "month":
		start = time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, -1)
	case "quarter":
		start = time.Date(date.Year(), time.Month((int(date.Month())-1)/3*3+1), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 3, -1)
	case "year":
		start = time.Date(date.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(1, 0, -1)
	case "default", "full", "":
		start = monday.AddDate(0, 0, -7)
		end = monday.AddDate(0, 0, 48)
	default:
		return start, end, failure(422, "invalid_period", "Choose a supported calendar period")
	}
	if start.Year() < 1 || end.Year() > 9999 {
		return start, end, failure(422, "invalid_date", "Calendar bounds must fit four-digit dates")
	}
	return start, end, nil
}
func readCalendar(st *model.State, a model.Account, in CalendarInput) (Calendar, error) {
	result := Calendar{Rows: []CalendarRow{}}
	if !contains([]string{"", "default", "week", "month", "quarter", "year", "full"}, in.Period) {
		return result, failure(422, "invalid_period", "Choose a supported calendar period")
	}
	date, err := reviewCalendarDate(in.Date)
	if err != nil {
		return result, err
	}
	start, end, err := dateWindow(date, in.Period)
	if err != nil {
		return result, err
	}
	if in.MaxRows == 0 {
		in.MaxRows = 500
	}
	if in.MaxRows < 1 || in.MaxRows > 1000 || len(in.Query) > 500 {
		return result, failure(422, "invalid_calendar_filter", "Use a query within 500 bytes and 1–1,000 rows")
	}
	if in.ProjectID != 0 {
		if err := require(st, a, in.ProjectID, "Viewer"); err != nil {
			return result, err
		}
	}
	var dueStart, dueEnd string
	if in.DuePeriod != "" {
		if !contains([]string{"today", "week", "month", "quarter", "year"}, in.DuePeriod) {
			return result, failure(422, "invalid_due_period", "Choose a supported due-date preset")
		}
		left, right, err := dateWindow(date, in.DuePeriod)
		if err != nil {
			return result, err
		}
		dueStart, dueEnd = left.Format("2006-01-02"), right.Format("2006-01-02")
	}
	allowed := map[int64]model.Ticket{}
	children := map[int64][]int64{}
	for id, t := range st.Tickets {
		if role(st, a, t.ProjectID) != "" && (in.ProjectID == 0 || t.ProjectID == in.ProjectID) {
			allowed[id] = t
			children[t.ParentID] = append(children[t.ParentID], id)
		}
	}
	cache := map[int64]calendarBounds{}
	visiting := map[int64]bool{}
	var bounds func(int64) calendarBounds
	bounds = func(id int64) calendarBounds {
		if b, ok := cache[id]; ok {
			return b
		}
		if visiting[id] {
			return calendarBounds{}
		}
		visiting[id] = true
		t := allowed[id]
		b := calendarBounds{start: t.Start, end: t.ETA}
		if b.start == "" {
			b.start = b.end
		}
		if b.end == "" {
			b.end = b.start
		}
		for _, childID := range children[id] {
			if allowed[childID].Deleted {
				continue
			}
			child := bounds(childID)
			if child.start != "" && (b.start == "" || child.start < b.start) {
				b.start = child.start
				b.derived = true
			}
			if child.end != "" && (b.end == "" || child.end > b.end) {
				b.end = child.end
				b.derived = true
			}
		}
		if b.start == "" && t.Due != "" {
			b.start, b.end, b.milestone = t.Due, t.Due, true
		}
		visiting[id] = false
		cache[id] = b
		return b
	}
	selected := map[int64]bool{}
	context := map[int64]bool{}
	for id, t := range allowed {
		if t.Archived || t.Deleted || !textMatches(t.Title+" "+t.Description+" "+strings.Join(t.Labels, " "), in.Query) || in.DuePeriod != "" && (t.Due == "" || t.Due < dueStart || t.Due > dueEnd) {
			continue
		}
		b := bounds(id)
		if b.start == "" {
			result.Unscheduled++
			continue
		}
		if in.Period != "full" && !(b.end >= start.Format("2006-01-02") && b.start <= end.Format("2006-01-02")) && !(t.Due != "" && t.Due >= start.Format("2006-01-02") && t.Due <= end.Format("2006-01-02")) {
			continue
		}
		selected[id] = true
	}
	for id := range selected {
		parent := allowed[id].ParentID
		seen := map[int64]bool{}
		for parent != 0 && !seen[parent] {
			seen[parent] = true
			t, ok := allowed[parent]
			if !ok {
				break
			}
			if !selected[parent] {
				context[parent] = true
			}
			parent = t.ParentID
		}
	}
	for id := range context {
		selected[id] = true
	}
	if in.Period == "full" && len(selected) > 0 {
		first, last := "", ""
		for id := range selected {
			b := bounds(id)
			if b.start != "" && (first == "" || b.start < first) {
				first = b.start
			}
			if b.end != "" && (last == "" || b.end > last) {
				last = b.end
			}
			if due := allowed[id].Due; due != "" {
				if first == "" || due < first {
					first = due
				}
				if last == "" || due > last {
					last = due
				}
			}
		}
		if first != "" {
			start, _ = reviewCalendarDate(first)
			end, _ = reviewCalendarDate(last)
		}
	}
	result.Start, result.End = start.Format("2006-01-02"), end.Format("2006-01-02")
	priority := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	for parent := range children {
		sort.Slice(children[parent], func(x, y int) bool {
			a, b := allowed[children[parent][x]], allowed[children[parent][y]]
			if a.ProjectID != b.ProjectID {
				return a.ProjectID < b.ProjectID
			}
			if a.Priority != b.Priority {
				return priority[a.Priority] < priority[b.Priority]
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.After(b.CreatedAt)
			}
			return a.ID < b.ID
		})
	}
	visited := map[int64]bool{}
	var appendRows func(int64)
	appendRows = func(id int64) {
		if visited[id] {
			return
		}
		visited[id] = true
		if selected[id] {
			t := allowed[id]
			b := bounds(id)
			if context[id] || b.start == "" || b.end >= result.Start && b.start <= result.End || t.Due != "" && t.Due >= result.Start && t.Due <= result.End {
				result.Rows = append(result.Rows, CalendarRow{TicketID: id, ProjectID: t.ProjectID, ParentID: t.ParentID, Title: t.Title, Kind: t.Kind, Status: t.Status, Progress: t.Progress, Start: b.start, End: b.end, ExplicitStart: t.Start, ExplicitETA: t.ETA, Due: t.Due, Derived: b.derived, Milestone: b.milestone, Context: context[id]})
			}
		}
		for _, child := range children[id] {
			appendRows(child)
		}
	}
	for _, id := range children[0] {
		appendRows(id)
	}
	if len(result.Rows) > in.MaxRows {
		return Calendar{}, failure(422, "calendar_too_large", "Narrow project/search/due scope before loading more rows")
	}
	return result, nil
}
func planningOperations(st *model.State, a model.Account, operation string, raw json.RawMessage) (any, bool, error) {
	switch operation {
	case "searchTickets":
		var in ListInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		result, err := searchTickets(st, a, in)
		return result, false, err
	case "getCalendar":
		var in CalendarInput
		if err := parse(raw, &in); err != nil {
			return nil, false, err
		}
		result, err := readCalendar(st, a, in)
		return result, false, err
	}
	return welcomeOperations(st, a, operation, raw)
}
