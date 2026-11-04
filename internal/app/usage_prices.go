package app

import (
	"context"
	"encoding/json"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"teleport/internal/model"
)

type PublishUsagePriceInput struct {
	Model           string `json:"model"`
	ExpectedVersion int64  `json:"expectedVersion"`
	RequestID       string `json:"requestId"`
	Effective       string `json:"effective"`
	Source          string `json:"source"`
	Input           string `json:"input"`
	Cached          string `json:"cached"`
	Output          string `json:"output"`
}
type UsagePriceList struct {
	Items []model.UsagePrice `json:"items"`
}
type UsagePriceHistoryInput struct {
	Model  string `json:"model"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type UsageValuationHistoryInput struct {
	ID     string `json:"id"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func usdMicros(value string) (int64, bool) {
	p := strings.Split(value, ".")
	if len(p) > 2 || p[0] == "" || len(p[0]) > 6 {
		return 0, false
	}
	for _, part := range p {
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return 0, false
			}
		}
	}
	whole, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil || whole < 0 {
		return 0, false
	}
	fraction := ""
	if len(p) == 2 {
		fraction = p[1]
		if len(fraction) > 6 {
			return 0, false
		}
	}
	fraction += strings.Repeat("0", 6-len(fraction))
	part, e := strconv.ParseInt(fraction, 10, 64)
	if e != nil {
		return 0, false
	}
	return whole*1_000_000 + part, true
}
func priceKey(modelName string) string { return "codex:" + modelName }
func latestPrice(st *model.State, name string) (model.UsagePrice, bool) {
	rows := st.UsagePrices[priceKey(name)]
	if len(rows) == 0 {
		return model.UsagePrice{}, false
	}
	return rows[len(rows)-1], true
}
func applyUsagePrice(st *model.State, row *model.LocalSession) bool {
	if row.Spellcheck == nil || row.Spellcheck.Usage == nil || row.Persona == nil || row.Persona.Harness != "codex" || row.Persona.Model == "" {
		return false
	}
	u := row.Spellcheck.Usage
	if u.Source != "codex-exec-json" || u.Cached == nil {
		return false
	}
	p, ok := latestPrice(st, row.Persona.Model)
	if !ok {
		return false
	}
	values := row.Spellcheck.Valuations
	if len(values) > 0 && values[len(values)-1].PriceVersion >= p.Version {
		return false
	}
	sum := new(big.Int)
	for _, pair := range [][2]int64{{u.Input - *u.Cached, p.InputMicros}, {*u.Cached, p.CachedMicros}, {u.Output, p.OutputMicros}} {
		sum.Add(sum, new(big.Int).Mul(big.NewInt(pair[0]), big.NewInt(pair[1])))
	}
	sum.Quo(sum, big.NewInt(1_000_000))
	if !sum.IsInt64() {
		return false
	}
	var prior *int64
	if len(values) > 0 {
		value := values[len(values)-1].EstimateMicros
		prior = &value
	}
	row.Spellcheck.Valuations = append(values, model.UsageValuation{PriceVersion: p.Version, EstimateMicros: sum.Int64(), PreviousMicros: prior, InputTokens: u.Input, CachedTokens: *u.Cached, OutputTokens: u.Output, Calculation: "codex-usd-v1", At: time.Now().UTC(), Effective: p.Effective, Source: p.Source})
	return true
}
func (s *Service) usagePriceCall(ctx context.Context, token, op string, raw json.RawMessage) (any, error) {
	if op == "listUsagePriceHistory" || op == "listUsageValuations" {
		var modelName, id, cursorValue string
		var limitValue int
		if op == "listUsagePriceHistory" {
			var in UsagePriceHistoryInput
			if err := parse(raw, &in); err != nil {
				return nil, err
			}
			modelName, cursorValue, limitValue = in.Model, in.Cursor, in.Limit
		} else {
			var in UsageValuationHistoryInput
			if err := parse(raw, &in); err != nil {
				return nil, err
			}
			id, cursorValue, limitValue = in.ID, in.Cursor, in.Limit
		}
		after, limit, err := pageBounds(cursorValue, limitValue)
		if err != nil {
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
		if op == "listUsagePriceHistory" {
			if !a.Administrator {
				return nil, failure(403, "administrator_required", "Price history requires an administrator")
			}
			out := Page[model.UsagePrice]{Items: []model.UsagePrice{}}
			for _, v := range st.UsagePrices[priceKey(modelName)] {
				if v.Version <= after {
					continue
				}
				if len(out.Items) == limit {
					out.NextCursor = cursor(out.Items[len(out.Items)-1].Version)
					break
				}
				out.Items = append(out.Items, v)
			}
			return out, nil
		}
		row, ok := st.LocalSessions[id]
		if !ok || row.Spellcheck == nil {
			return nil, failure(404, "not_found", "Usage history not found")
		}
		if err = localAllowed(st, a, row); err != nil {
			return nil, err
		}
		out := Page[model.UsageValuation]{Items: []model.UsageValuation{}}
		for i, v := range row.Spellcheck.Valuations {
			n := int64(i + 1)
			if n <= after {
				continue
			}
			if len(out.Items) == limit {
				out.NextCursor = cursor(n - 1)
				break
			}
			out.Items = append(out.Items, v)
		}
		return out, nil
	}
	if op == "listUsagePrices" {
		if err := parse(raw, &struct{}{}); err != nil {
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
		if !a.Administrator {
			return nil, failure(403, "administrator_required", "Price configuration requires an administrator")
		}
		out := UsagePriceList{Items: []model.UsagePrice{}}
		for _, rows := range st.UsagePrices {
			if len(rows) > 0 {
				out.Items = append(out.Items, rows[len(rows)-1])
			}
		}
		sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Model < out.Items[j].Model })
		return out, nil
	}
	var in PublishUsagePriceInput
	if err := parse(raw, &in); err != nil {
		return nil, err
	}
	in.Model = strings.TrimSpace(in.Model)
	in.Source = strings.TrimSpace(in.Source)
	input, ok1 := usdMicros(in.Input)
	cached, ok2 := usdMicros(in.Cached)
	output, ok3 := usdMicros(in.Output)
	date, err := time.Parse("2006-01-02", in.Effective)
	if err != nil || date.Year() < 1 || in.Model == "" || len(in.Model) > 100 || len(in.Source) == 0 || len(in.Source) > 500 || strings.ContainsRune(in.Model, 0) || strings.ContainsRune(in.Source, 0) || !ok1 || !ok2 || !ok3 {
		return nil, failure(422, "invalid_price", "Provide a model, effective date, source and nonnegative USD rates per million tokens")
	}
	var result model.UsagePrice
	err = s.db.Update(ctx, func(st *model.State) (bool, error) {
		collections(st)
		a, e := actor(st, token)
		if e != nil {
			return false, e
		}
		if !a.Administrator || a.ServiceRunID != "" || a.ServiceSessionID != "" {
			return false, failure(403, "administrator_required", "Only an administrator may publish prices")
		}
		key, hash, prior, e := receipt(st, a, op, in.RequestID, in)
		if e != nil {
			return false, e
		}
		if prior != nil {
			if json.Unmarshal(prior, &result) != nil {
				return false, failure(500, "invalid_receipt", "Invalid price receipt")
			}
			return false, nil
		}
		if st.UsagePrices == nil {
			st.UsagePrices = map[string][]model.UsagePrice{}
		}
		if st.PriceRecalculations == nil {
			st.PriceRecalculations = map[string]model.PriceRecalculation{}
		}
		k := priceKey(in.Model)
		rows := st.UsagePrices[k]
		version := int64(len(rows))
		if version != in.ExpectedVersion {
			return false, failure(409, "version_conflict", "Price changed; reload before publishing")
		}
		if len(rows) >= 100 || len(st.UsagePrices) >= 100 && len(rows) == 0 {
			return false, failure(409, "price_capacity", "The price catalogue is full")
		}
		result = model.UsagePrice{Harness: "codex", Model: in.Model, Version: version + 1, Effective: in.Effective, Source: in.Source, InputMicros: input, CachedMicros: cached, OutputMicros: output, PublishedAt: time.Now().UTC(), PublishedBy: a.ID}
		st.UsagePrices[k] = append(rows, result)
		st.PriceRecalculations[k] = model.PriceRecalculation{Version: result.Version}
		remember(st, key, hash, result)
		return true, nil
	})
	return result, err
}
func (s *Service) PumpUsagePrices(ctx context.Context) error {
	return s.db.Update(ctx, func(st *model.State) (bool, error) {
		for key, job := range st.PriceRecalculations {
			if job.Done {
				continue
			}
			rows := st.UsagePrices[key]
			if len(rows) == 0 || rows[len(rows)-1].Version != job.Version {
				job.Done = true
				st.PriceRecalculations[key] = job
				return true, nil
			}
			ids := []int64{}
			byNum := map[int64]string{}
			for id, row := range st.LocalSessions {
				if row.Number > job.Cursor {
					ids = append(ids, row.Number)
					byNum[row.Number] = id
				}
			}
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			if len(ids) > 25 {
				ids = ids[:25]
			}
			for _, n := range ids {
				row := st.LocalSessions[byNum[n]]
				if row.Persona != nil && priceKey(row.Persona.Model) == key && applyUsagePrice(st, &row) {
					st.LocalSessions[row.ID] = row
				}
				job.Cursor = n
			}
			if len(ids) < 25 {
				job.Done = true
			}
			st.PriceRecalculations[key] = job
			return true, nil
		}
		return false, nil
	})
}
