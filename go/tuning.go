package guardrail

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Offline threshold tuning, replayed against recorded answers.
//
// Calling Jev is the expensive part of calibration: tokens, rate-limit budget, and the labelled
// set itself, which is the scarcest input of all. Decide is a pure function, so once the answers
// for a case are on disk, every later question about thresholds can be answered without a
// network call. The flow is: run the labelled set once with a RecordingTransport, then sweep here.

// Record is one recorded exchange, plus whatever label the case carried.
type Record struct {
	Surface Surface
	Answers Answers
	// Expected is the labelled action, or "" when the case carries none.
	Expected Action
	// ExpectedCategory is the hazard the case is meant to exercise, when the labelled set says.
	// Separation needs it: without it there is no way to tell a case this category should have
	// caught from one that belongs to a different category entirely.
	ExpectedCategory string
	ID               string
	State            any
}

// RecordFromJSON reads one row as written by a recording run. expectField defaults to
// "expected_action".
func RecordFromJSON(row map[string]any, expectField string) Record {
	if expectField == "" {
		expectField = "expected_action"
	}
	input := mapOf(row["input"])
	pick := func(key string) string {
		if v := stringOr(row[key], ""); v != "" {
			return v
		}
		return stringOr(input[key], "")
	}
	answers := Answers{}
	for name, a := range mapOf(row["answers"]) {
		answers[name] = Answer(mapOf(a))
	}
	surface := Surface(stringOr(row["surface"], ""))
	if surface == "" {
		surface = SurfaceInput
	}
	return Record{
		Surface:          surface,
		Answers:          answers,
		Expected:         Action(pick(expectField)),
		ExpectedCategory: pick("expected_category"),
		ID:               pick("id"),
		State:            row["state"],
	}
}

// LoadRecords reads a JSONL file of recorded exchanges.
func LoadRecords(path, expectField string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var records []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal([]byte(text), &row); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		records = append(records, RecordFromJSON(row, expectField))
	}
	return records, scanner.Err()
}

// Replay re-decides every record under a policy. No network, no model.
func Replay(p *Policy, records []Record) []Verdict {
	out := make([]Verdict, len(records))
	for i, r := range records {
		out[i] = Decide(p, r.Surface, r.Answers, DecideOptions{})
	}
	return out
}

// Report is how a policy scores against a labelled set.
//
// Exact match is the least interesting number here. A guardrail is judged on its two kinds of
// error separately, because they are not interchangeable: letting harmful content through is a
// safety failure, blocking ordinary users is a product failure, and the review queue is what a
// team has to staff.
type Report struct {
	Total          int
	Exact          int
	Under          int
	Over           int
	CriticalMisses []string
	OverBlocks     []string
	Confusion      map[Action]map[Action]int
	Actions        map[Action]int
	Degraded       int
}

func (r *Report) rate(n int) float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(n) / float64(r.Total)
}

// ExactRate is the share of labelled cases decided exactly as labelled.
func (r *Report) ExactRate() float64 { return r.rate(r.Exact) }

// ReviewRate is the share of cases sent to review.
func (r *Report) ReviewRate() float64 { return r.rate(r.Actions[Review]) }

// BlockRate is the share of cases blocked.
func (r *Report) BlockRate() float64 { return r.rate(r.Actions[Block]) }

// MarshalJSON writes the same shape as the Python package's Report.as_dict.
func (r *Report) MarshalJSON() ([]byte, error) {
	confusion := r.Confusion
	if confusion == nil {
		confusion = map[Action]map[Action]int{}
	}
	return json.Marshal(map[string]any{
		"total":           r.Total,
		"exact":           r.Exact,
		"exact_rate":      round(r.ExactRate(), 4),
		"under_enforced":  r.Under,
		"over_enforced":   r.Over,
		"critical_misses": nonNil(r.CriticalMisses),
		"over_blocks":     nonNil(r.OverBlocks),
		"review_rate":     round(r.ReviewRate(), 4),
		"block_rate":      round(r.BlockRate(), 4),
		"confusion":       confusion,
		"degraded":        r.Degraded,
	})
}

// Score compares replayed verdicts against the labels.
func Score(records []Record, verdicts []Verdict) *Report {
	report := &Report{Confusion: map[Action]map[Action]int{}, Actions: map[Action]int{}}
	for i := 0; i < len(records) && i < len(verdicts); i++ {
		record, verdict := records[i], verdicts[i]
		report.Total++
		report.Actions[verdict.Action]++
		if verdict.Degraded {
			report.Degraded++
		}
		if record.Expected == "" {
			continue
		}
		row := report.Confusion[record.Expected]
		if row == nil {
			row = map[Action]int{}
			report.Confusion[record.Expected] = row
		}
		row[verdict.Action]++

		id := record.ID
		if id == "" {
			id = "?"
		}
		got, want := Rank(verdict.Action), Rank(record.Expected)
		switch {
		case got == want:
			report.Exact++
		case got < want:
			report.Under++
			// The label said hold it and the policy would have delivered it: the failure that
			// matters, listed by name rather than counted.
			if record.Expected == Block && (verdict.Action == Allow || verdict.Action == Flag) {
				report.CriticalMisses = append(report.CriticalMisses, id)
			}
		default:
			report.Over++
			if record.Expected == Allow && verdict.Action == Block {
				report.OverBlocks = append(report.OverBlocks, id)
			}
		}
	}
	return report
}

// BandNames are the three threshold bands, from strictest.
var BandNames = []string{"block", "review", "flag"}

// Override returns a copy of a policy with thresholds replaced.
//
// Keys are "category.surface.band", for example "prv.output.review". Use "default" as the
// surface to change the fallback band. The bands are kept ordered, so an override cannot
// produce a policy that will not load.
func Override(p *Policy, changes map[string]float64) (*Policy, error) {
	var categories map[string]map[string]any
	if err := json.Unmarshal(p.raw["categories"], &categories); err != nil {
		return nil, fmt.Errorf("policy has no categories to override: %w", err)
	}
	// Apply in a stable order, so the same changes always produce the same policy.
	axes := make([]string, 0, len(changes))
	for axis := range changes {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	for _, axis := range axes {
		category, surface, band, err := parseAxis(axis)
		if err != nil {
			return nil, err
		}
		cat, ok := categories[category]
		if !ok {
			return nil, fmt.Errorf("unknown category %q in %q", category, axis)
		}
		thresholds := mapOf(cat["thresholds"])
		bands := copyMap(mapOf(thresholds[surface]))
		if len(bands) == 0 {
			bands = copyMap(mapOf(thresholds["default"]))
		}
		if len(bands) == 0 {
			return nil, fmt.Errorf("category %q has no thresholds to override", category)
		}
		bands[band] = changes[axis]
		block := floatOr(bands["block"], 0)
		review := min(floatOr(bands["review"], 0), block)
		flag := min(floatOr(bands["flag"], 0), review)
		thresholds[surface] = map[string]any{"block": block, "review": review, "flag": flag}
		cat["thresholds"] = thresholds
	}

	raw := make(map[string]json.RawMessage, len(p.raw))
	for k, v := range p.raw {
		raw[k] = v
	}
	encoded, err := json.Marshal(categories)
	if err != nil {
		return nil, err
	}
	raw["categories"] = encoded
	return fromRaw(raw)
}

// SweepRow is one candidate value and how the policy scored with it.
type SweepRow struct {
	Value  float64
	Report *Report
}

// Sweep scores the set once per candidate value of one threshold.
func Sweep(p *Policy, records []Record, axis string, values []float64) ([]SweepRow, error) {
	var rows []SweepRow
	for _, value := range values {
		candidate, err := Override(p, map[string]float64{axis: value})
		if err != nil {
			return nil, err
		}
		rows = append(rows, SweepRow{Value: value, Report: Score(records, Replay(candidate, records))})
	}
	return rows, nil
}

// Distribution summarises a set of probabilities.
type Distribution struct {
	N      int     `json:"n"`
	Min    float64 `json:"min"`
	P25    float64 `json:"p25"`
	Median float64 `json:"median"`
	P75    float64 `json:"p75"`
	Max    float64 `json:"max"`
}

// MarshalJSON writes only the count for an empty group, as the Python package does.
func (d Distribution) MarshalJSON() ([]byte, error) {
	if d.N == 0 {
		return []byte(`{"n":0}`), nil
	}
	type plain Distribution
	return json.Marshal(plain(d))
}

// SeparationResult says how far apart a category's probabilities are between the cases it should
// and should not catch.
type SeparationResult struct {
	Category      string       `json:"category"`
	Surface       Surface      `json:"surface"`
	Basis         string       `json:"basis"`
	Thresholds    *Bands       `json:"thresholds"`
	ShouldFire    Distribution `json:"should_fire"`
	ShouldNotFire Distribution `json:"should_not_fire"`
	Separated     bool         `json:"separated"`
}

// Separation is the question a threshold actually answers. Where the two groups overlap, no
// number separates them and the fix is the category's description, which is the text Jev reads.
//
// The split is by ExpectedCategory when the labelled set carries one. Falling back to the action
// label is much weaker: it counts every non-allow case as one this category should have caught,
// so a self-harm case would be held against the fraud category and everything would look like an
// overlap. The result says which basis was used.
func Separation(p *Policy, records []Record, category string, surface Surface) SeparationResult {
	var relevant []Record
	basis := "action"
	for _, r := range records {
		if r.Surface == surface && r.Expected != "" {
			relevant = append(relevant, r)
			if r.ExpectedCategory != "" {
				basis = "category"
			}
		}
	}

	var should, shouldNot []float64
	for _, r := range relevant {
		probability := ProbabilityOf(r, category)
		var fires bool
		if basis == "category" {
			fires = r.ExpectedCategory == category
		} else {
			fires = Rank(r.Expected) >= Rank(Review)
		}
		if fires {
			should = append(should, probability)
		} else {
			shouldNot = append(shouldNot, probability)
		}
	}

	result := SeparationResult{
		Category:      category,
		Surface:       surface,
		Basis:         basis,
		ShouldFire:    describe(should),
		ShouldNotFire: describe(shouldNot),
	}
	if cat, ok := p.Categories[category]; ok {
		if bands, has := cat.Threshold(surface); has {
			result.Thresholds = &bands
		}
	}
	if len(should) > 0 && len(shouldNot) > 0 {
		result.Separated = minOf(should) > maxOf(shouldNot)
	}
	return result
}

// ProbabilityOf is the probability a replay would use for a category: the higher of choice and
// sentinel.
func ProbabilityOf(r Record, category string) float64 {
	value := 0.0
	if category != NoneLabel {
		value = floatOr(mapOf(r.Answers[Hazard]["probabilities"])[category], 0)
	}
	if sentinel := r.Answers[SentinelPrefix+category]; len(sentinel) > 0 {
		value = max(value, floatOr(sentinel["noul"], 0))
	}
	return value
}

// Grid is an inclusive float range, rounded so the values print cleanly.
func Grid(start, stop, step float64) ([]float64, error) {
	if step <= 0 {
		return nil, fmt.Errorf("step must be positive")
	}
	var values []float64
	for current := start; current <= stop+1e-9; current += step {
		values = append(values, round(current, 4))
	}
	return values, nil
}

func parseAxis(axis string) (string, string, string, error) {
	parts := strings.Split(axis, ".")
	if len(parts) != 3 || (parts[2] != "block" && parts[2] != "review" && parts[2] != "flag") {
		return "", "", "", fmt.Errorf("axis must be category.surface.band with band in %v, got %q", BandNames, axis)
	}
	return parts[0], parts[1], parts[2], nil
}

func describe(values []float64) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	n := len(ordered)
	median := ordered[n/2]
	if n%2 == 0 {
		median = (ordered[n/2-1] + ordered[n/2]) / 2
	}
	return Distribution{
		N:      n,
		Min:    round(ordered[0], 4),
		P25:    round(ordered[n/4], 4),
		Median: round(median, 4),
		P75:    round(ordered[(3*n)/4], 4),
		Max:    round(ordered[n-1], 4),
	}
}

func minOf(values []float64) float64 {
	out := values[0]
	for _, v := range values[1:] {
		out = min(out, v)
	}
	return out
}

func maxOf(values []float64) float64 {
	out := values[0]
	for _, v := range values[1:] {
		out = max(out, v)
	}
	return out
}
