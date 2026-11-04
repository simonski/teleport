package app

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"teleport/internal/model"
)

type UsageSummaryInput struct {
	ProjectID int64 `json:"projectId,omitempty"`
}
type UsageWindow struct {
	Name                string             `json:"name"`
	Since               *time.Time         `json:"since,omitempty"`
	Invocations         int                `json:"invocations"`
	Succeeded           int                `json:"succeeded"`
	Failed              int                `json:"failed"`
	Pending             int                `json:"pending"`
	InteractiveSessions int                `json:"interactiveSessions"`
	UnknownOutcomes     int                `json:"unknownOutcomes"`
	MeasuredRuns        int                `json:"measuredRuns"`
	MissingRuns         int                `json:"missingRuns"`
	MissingCached       int                `json:"missingCached"`
	Input               *int64             `json:"input"`
	Cached              *int64             `json:"cached"`
	Output              *int64             `json:"output"`
	Tokens              *int64             `json:"tokens"`
	EstimatedUSDMicros  *int64             `json:"estimatedUsdMicros"`
	EstimatedRuns       int                `json:"estimatedRuns"`
	PricePending        int                `json:"pricePending"`
	FutureRates         bool               `json:"futureRates"`
	RevisedRates        bool               `json:"revisedRates"`
	PriceDetails        []UsagePriceDetail `json:"priceDetails"`
	ElapsedSeconds      float64            `json:"elapsedSeconds"`
	ElapsedPartial      bool               `json:"elapsedPartial"`
}
type UsagePriceDetail struct {
	Model     string `json:"model"`
	Version   int64  `json:"version"`
	Effective string `json:"effective"`
	Source    string `json:"source"`
}
type UsageSummary struct {
	ProjectID int64         `json:"projectId"`
	AsOf      time.Time     `json:"asOf"`
	Windows   []UsageWindow `json:"windows"`
}

func usageWindow(st *model.State, a model.Account, project int64, name string, since time.Time, now time.Time) UsageWindow {
	result := UsageWindow{Name: name, PriceDetails: []UsagePriceDetail{}}
	priceSeen := map[string]bool{}
	if !since.IsZero() {
		v := since
		result.Since = &v
	}
	var input, cached, output, cost int64
	for _, row := range st.LocalSessions {
		if row.Harness == "shell" || role(st, a, row.ProjectID) == "" || project != 0 && row.ProjectID != project || row.CreatedAt.After(now) || !since.IsZero() && row.CreatedAt.Before(since) {
			continue
		}
		if row.Spellcheck == nil && row.Managed == nil && row.ProposalRun == nil {
			result.InteractiveSessions++
			continue
		}
		if row.State == "preparing" && row.ProcessID == 0 {
			result.Pending++
			continue
		}
		result.Invocations++
		switch {
		case row.ProposalRun != nil && row.State == "exited" && row.ExitCode != nil && *row.ExitCode == 0 && !row.StopRequested && row.ProposalRun.ProposalID != "":
			result.Succeeded++
		case row.ProposalRun != nil && (row.State == "failed" || row.State == "exited"):
			result.Failed++
		case row.Spellcheck != nil && row.Spellcheck.State == "completed":
			result.Succeeded++
		case row.Spellcheck != nil && row.Spellcheck.State == "failed":
			result.Failed++
		case row.Managed != nil && row.Managed.Applied && row.Managed.Error != "":
			result.Failed++
		case row.Managed != nil && row.Managed.Applied && row.Managed.Result != nil && row.Managed.Result.Outcome == "success":
			result.Succeeded++
		case row.Managed != nil && row.Managed.Applied && row.Managed.Result != nil && row.Managed.Result.Outcome == "failure":
			result.Failed++
		case activeLocal(row):
			result.Pending++
		default:
			result.UnknownOutcomes++
		}
		if row.ExitedAt != nil && !row.ExitedAt.Before(row.CreatedAt) {
			result.ElapsedSeconds += row.ExitedAt.Sub(row.CreatedAt).Seconds()
		} else {
			result.ElapsedPartial = true
		}
		var u *model.TokenEvidence
		values := []model.UsageValuation{}
		if row.Spellcheck != nil {
			u, values = row.Spellcheck.Usage, row.Spellcheck.Valuations
		} else if row.ProposalRun != nil {
			u, values = row.ProposalRun.Usage, row.ProposalRun.Valuations
		}
		if u != nil {
			result.MeasuredRuns++
			input += u.Input
			if u.Cached != nil {
				cached += *u.Cached
			} else {
				result.MissingCached++
			}
			output += u.Output
			if u.EstimateUSDMicros != nil {
				cost += *u.EstimateUSDMicros
				result.EstimatedRuns++
			} else if row.Persona != nil && row.Persona.Harness == "codex" && row.Persona.Model != "" && u.Cached != nil {
				price, ok := latestPrice(st, row.Persona.Model)
				if ok && (len(values) == 0 || values[len(values)-1].PriceVersion != price.Version) {
					result.PricePending++
				}
				if len(values) > 0 {
					v := values[len(values)-1]
					cost += v.EstimateMicros
					result.EstimatedRuns++
					if v.Effective > now.UTC().Format("2006-01-02") {
						result.FutureRates = true
					}
					if len(values) > 1 {
						result.RevisedRates = true
					}
					key := row.Persona.Model + ":" + strconv.FormatInt(v.PriceVersion, 10)
					if !priceSeen[key] {
						priceSeen[key] = true
						result.PriceDetails = append(result.PriceDetails, UsagePriceDetail{Model: row.Persona.Model, Version: v.PriceVersion, Effective: v.Effective, Source: v.Source})
					}
				}
			}
		} else {
			result.MissingRuns++
		}
	}
	if result.MeasuredRuns > 0 {
		total := input + output
		result.Input = &input
		if result.MissingCached < result.MeasuredRuns {
			result.Cached = &cached
		}
		result.Output = &output
		result.Tokens = &total
	}
	if result.EstimatedRuns > 0 {
		result.EstimatedUSDMicros = &cost
	}
	sort.Slice(result.PriceDetails, func(i, j int) bool {
		if result.PriceDetails[i].Model == result.PriceDetails[j].Model {
			return result.PriceDetails[i].Version < result.PriceDetails[j].Version
		}
		return result.PriceDetails[i].Model < result.PriceDetails[j].Model
	})
	return result
}

func (s *Service) usageSummary(ctx context.Context, token string, raw json.RawMessage) (any, error) {
	var in UsageSummaryInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	st, err := s.db.Read(ctx)
	if err != nil {
		return nil, err
	}
	a, err := directoryActor(st, token)
	if err != nil {
		return nil, err
	}
	if in.ProjectID != 0 {
		if err = require(st, a, in.ProjectID, "Viewer"); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	out := UsageSummary{ProjectID: in.ProjectID, AsOf: now, Windows: []UsageWindow{}}
	for _, window := range []struct {
		name     string
		duration time.Duration
	}{{"hour", time.Hour}, {"day", 24 * time.Hour}, {"week", 7 * 24 * time.Hour}, {"month", 30 * 24 * time.Hour}, {"all", 0}} {
		since := time.Time{}
		if window.duration != 0 {
			since = now.Add(-window.duration)
		}
		out.Windows = append(out.Windows, usageWindow(st, a, in.ProjectID, window.name, since, now))
	}
	return out, nil
}
