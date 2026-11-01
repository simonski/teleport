package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"teleport/internal/app"
	"teleport/internal/model"
)

func TestSearchRanksTitleBeforeMetadataAndUsesUnicodeTokens(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Search", RequestID: "p"}).(model.Project)
	for n, record := range []struct{ title, description string }{{"Boarding plan", ""}, {"Boarding plan draft", ""}, {"A different request", "boarding plan"}, {"Straße checklist", " boarding plan "}} {
		call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: record.title, Description: record.description, RequestID: fmt.Sprintf("t%d", n)})
	}
	page := call(t, svc, token, "searchTickets", app.ListInput{ProjectID: p.ID, Query: "boarding plan", Limit: 1}).(app.Page[app.SearchHit])
	if len(page.Items) != 1 || page.Items[0].Ticket.Title != "Boarding plan" || page.NextCursor == "" {
		t.Fatal("exact title did not outrank weaker matches")
	}
	next := call(t, svc, token, "searchTickets", app.ListInput{ProjectID: p.ID, Query: "boarding plan", Limit: 1, Cursor: page.NextCursor}).(app.Page[app.SearchHit])
	if len(next.Items) != 1 || next.Items[0].Ticket.Title != "Boarding plan draft" {
		t.Fatal("rank cursor lost prefix ordering")
	}
	unicode := call(t, svc, token, "searchTickets", app.ListInput{ProjectID: p.ID, Query: "STRASSE checklist"}).(app.Page[app.SearchHit])
	if len(unicode.Items) != 1 || unicode.Items[0].Ticket.Title != "Straße checklist" {
		t.Fatal("Unicode folding/token matching failed")
	}
	body, _ := json.Marshal(app.ListInput{ProjectID: p.ID, Query: "different", Cursor: page.NextCursor})
	if _, err := svc.Call(context.Background(), token, "searchTickets", body); err == nil {
		t.Fatal("cursor accepted a different query")
	}
}

func TestCalendarInclusiveDatesDerivedParentsAndDSTPresets(t *testing.T) {
	svc, token := fixture(t)
	p := call(t, svc, token, "createProject", app.CreateProjectInput{Name: "Calendar", RequestID: "p"}).(model.Project)
	ticket := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Cross the clock change", RequestID: "t"}).(model.Ticket)
	ticket = call(t, svc, token, "updateTicket", app.UpdateTicketInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Start: ptr("2026-03-28"), ETA: ptr("2026-03-30")}).(model.Ticket)
	calendar := call(t, svc, token, "getCalendar", app.CalendarInput{ProjectID: p.ID, Date: "2026-03-29", Period: "week"}).(app.Calendar)
	if calendar.Start != "2026-03-23" || calendar.End != "2026-03-29" || len(calendar.Rows) != 3 {
		t.Fatal("week/calendar hierarchy is not stable across DST")
	}
	for _, row := range calendar.Rows {
		if row.Start != "2026-03-28" || row.End != "2026-03-30" {
			t.Fatal("inclusive child/parent bounds changed dates")
		}
	}
	retained := call(t, svc, token, "getTicket", app.IDInput{ID: ticket.ID}).(model.Ticket)
	if retained.Version != ticket.Version {
		t.Fatal("calendar query changed ticket history")
	}
	calendar = call(t, svc, token, "getCalendar", app.CalendarInput{ProjectID: p.ID, Date: "2026-03-29", Period: "default"}).(app.Calendar)
	if calendar.Start != "2026-03-16" || calendar.End != "2026-05-10" {
		t.Fatal("default window did not include previous week and six following weeks")
	}
	sibling := call(t, svc, token, "createTicket", app.CreateTicketInput{ProjectID: p.ID, Title: "Other April work", RequestID: "sibling"}).(model.Ticket)
	call(t, svc, token, "updateTicket", app.UpdateTicketInput{ID: sibling.ID, ExpectedVersion: sibling.Version, Start: ptr("2026-04-08"), ETA: ptr("2026-04-08")})
	filtered := call(t, svc, token, "getCalendar", app.CalendarInput{ProjectID: p.ID, Date: "2026-04-08", Period: "week", Query: "clock"}).(app.Calendar)
	if len(filtered.Rows) != 0 {
		t.Fatal("out-of-period matches exposed unrelated context rows")
	}
	ticket = call(t, svc, token, "updateTicket", app.UpdateTicketInput{ID: ticket.ID, ExpectedVersion: ticket.Version, Due: ptr("2026-11-01")}).(model.Ticket)
	deadline := call(t, svc, token, "getCalendar", app.CalendarInput{ProjectID: p.ID, Date: "2026-11-01", Period: "month", DuePeriod: "today", Query: "clock"}).(app.Calendar)
	if len(deadline.Rows) != 3 {
		t.Fatal("current deadline disappeared because its older planned span is outside the window")
	}
	full := call(t, svc, token, "getCalendar", app.CalendarInput{ProjectID: p.ID, Date: "2026-11-01", Period: "full", Query: "clock"}).(app.Calendar)
	if full.End != "2026-11-01" {
		t.Fatal("full range omitted a deadline beyond its planned span")
	}
}
