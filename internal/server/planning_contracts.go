package server

import "teleport/internal/app"

func init() {
	Operations = append(Operations, Operation{"searchTickets", "ACT-0109", "Read authorized relevance-ranked live search pages"}, Operation{"getCalendar", "ACT-0610", "Read inclusive calendar-date spans without ticket mutations"})
	contracts["searchTickets"] = contract{app.ListInput{}, app.Page[app.SearchHit]{}}
	contracts["getCalendar"] = contract{app.CalendarInput{}, app.Calendar{}}
}
