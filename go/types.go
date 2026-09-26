package guardrail

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// Action is the enforcement decision.
type Action string

// Surface is the kind of content being checked.
type Surface string

// Route is how a deployment should handle content once the action is decided.
type Route string

const (
	Allow  Action = "allow"
	Flag   Action = "flag"
	Review Action = "review"
	Block  Action = "block"
)

const (
	SurfaceInput        Surface = "input"
	SurfaceOutput       Surface = "output"
	SurfaceConversation Surface = "conversation"
)

const (
	RouteDeliver       Route = "deliver"
	RouteRedact        Route = "redact"
	RouteGuide         Route = "guide"
	RouteCrisisSupport Route = "crisis_support"
	RouteHumanReview   Route = "human_review"
	RouteSafeResponse  Route = "safe_response"
)

// Surfaces lists every surface, in the order the checks run.
var Surfaces = []Surface{SurfaceInput, SurfaceOutput, SurfaceConversation}

// Ladder is the enforcement decision, ordered from least to most restrictive.
//
// Only four steps, and every one of them is a different answer to "does this content go out?".
// How to handle it once decided -- mask the personal data, steer the reply, hand it to a human --
// is a separate axis, carried by Verdict.Route. Keeping them apart is what lets a rule move a
// verdict one step without landing on a handling mode that makes no sense for the hazard.
var Ladder = []Action{Allow, Flag, Review, Block}

// withholding lists the routes that withhold the content and put something else in its place.
var withholding = map[Route]bool{RouteSafeResponse: true, RouteCrisisSupport: true, RouteHumanReview: true}

// Rank is the position of an action on the ladder; unknown actions sort as allow.
func Rank(a Action) int {
	for i, step := range Ladder {
		if step == a {
			return i
		}
	}
	return 0
}

// Stronger returns the more restrictive of two actions.
func Stronger(a, b Action) Action {
	if Rank(a) >= Rank(b) {
		return a
	}
	return b
}

// Weaker returns the less restrictive of two actions.
func Weaker(a, b Action) Action {
	if Rank(a) <= Rank(b) {
		return a
	}
	return b
}

// Shift moves an action along the ladder, clamped at both ends.
func Shift(a Action, steps int) Action {
	return Ladder[max(0, min(len(Ladder)-1, Rank(a)+steps))]
}

func validAction(a Action) bool {
	for _, step := range Ladder {
		if step == a {
			return true
		}
	}
	return false
}

// Finding is one hazard category that fired, with the evidence behind it.
type Finding struct {
	Category    string
	Name        string
	Probability float64
	Confidence  float64
	Action      Action
	Severity    float64
	Refs        []string
	Source      string
	Notes       []string
}

// MarshalJSON writes the same shape as the Python package's Finding.as_dict.
func (f Finding) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"category":    f.Category,
		"name":        f.Name,
		"probability": round(f.Probability, 4),
		"confidence":  round(f.Confidence, 4),
		"action":      f.Action,
		"severity":    round(f.Severity, 2),
		"refs":        nonNil(f.Refs),
		"source":      f.Source,
		"notes":       nonNil(f.Notes),
	})
}

// Usage is the token count Jev reported for one call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Verdict is the decision for one piece of content.
type Verdict struct {
	Action       Action
	Surface      Surface
	Findings     []Finding
	Signals      map[string]any
	Confidence   float64
	Severity     float64
	Route        Route
	AppliedRules []string
	Model        string
	Usage        Usage
	LatencyMS    float64
	// Degraded is true when Jev could not be reached, so nothing about the content was checked.
	Degraded bool
	// Error is set on a degraded verdict.
	Error    string
	PolicyID string
	// Cached is true when this verdict came from the cache rather than a fresh call.
	Cached bool
	// Partial is true when only the sentinel questions were asked, as mid-stream checks do.
	Partial bool
	// Prefilter names the rule when a prefilter decided without calling Jev.
	Prefilter string
	// Context is what the in-context output check found, when it ran.
	Context *ContextRead
}

// Allowed is true when the content may be delivered as-is or with a flag only.
func (v Verdict) Allowed() bool { return Rank(v.Action) <= Rank(Flag) }

// Blocked is true when the action is block.
func (v Verdict) Blocked() bool { return v.Action == Block }

// NeedsHuman is true when the action is review.
func (v Verdict) NeedsHuman() bool { return v.Action == Review }

// Deliverable is true when the content still reaches the user, possibly redacted or steered first.
func (v Verdict) Deliverable() bool { return !withholding[v.Route] }

// Top is the most serious finding, or nil when nothing fired.
func (v Verdict) Top() *Finding {
	if len(v.Findings) == 0 {
		return nil
	}
	return &v.Findings[0]
}

// Categories lists the categories that fired, most serious first.
func (v Verdict) Categories() []string {
	out := make([]string, len(v.Findings))
	for i, f := range v.Findings {
		out[i] = f.Category
	}
	return out
}

// HasCategory reports whether a category fired.
func (v Verdict) HasCategory(category string) bool {
	for _, f := range v.Findings {
		if f.Category == category {
			return true
		}
	}
	return false
}

// HasRule reports whether a rule was applied.
func (v Verdict) HasRule(rule string) bool {
	for _, r := range v.AppliedRules {
		if r == rule {
			return true
		}
	}
	return false
}

// MarshalJSON writes the same shape as the Python package's Verdict.as_dict.
func (v Verdict) MarshalJSON() ([]byte, error) {
	signals := v.Signals
	if signals == nil {
		signals = map[string]any{}
	}
	findings := v.Findings
	if findings == nil {
		findings = []Finding{}
	}
	return json.Marshal(map[string]any{
		"action":        v.Action,
		"allowed":       v.Allowed(),
		"surface":       v.Surface,
		"severity":      round(v.Severity, 2),
		"confidence":    round(v.Confidence, 4),
		"route":         v.Route,
		"findings":      findings,
		"signals":       signals,
		"applied_rules": nonNil(v.AppliedRules),
		"policy_id":     v.PolicyID,
		"model":         v.Model,
		"usage":         v.Usage,
		"latency_ms":    round(v.LatencyMS, 1),
		"degraded":      v.Degraded,
		"cached":        v.Cached,
		"partial":       v.Partial,
		"prefilter":     nullable(v.Prefilter),
		"error":         nullable(v.Error),
		"context":       v.Context,
	})
}

// Turn is one message in a conversation.
type Turn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// UnmarshalJSON accepts {"role", "content"} objects and ["role", "content"] pairs.
func (t *Turn) UnmarshalJSON(data []byte) error {
	var pair []any
	if err := json.Unmarshal(data, &pair); err == nil {
		if len(pair) != 2 {
			return fmt.Errorf("cannot read a conversation turn from %s", data)
		}
		t.Role, t.Content = pyStr(pair[0]), pyStr(pair[1])
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("cannot read a conversation turn from %s", data)
	}
	*t = turnFromMap(obj)
	return nil
}

func turnFromMap(m map[string]any) Turn {
	role, _ := m["role"].(string)
	if role == "" {
		role = "user"
	}
	content := ""
	if c, ok := m["content"]; ok && c != nil {
		content = pyStr(c)
	}
	return Turn{Role: role, Content: content}
}

// Error means the guardrail could not reach a verdict.
type Error struct {
	Msg string
	Err error
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Err }

func errorf(cause error, format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...), Err: cause}
}

// -- small helpers shared across the package ----------------------------

// round matches Python's round(): it rounds the exact binary value, and an exact half to even.
// math.Round would send 0.0625 to 0.063 where Python gives 0.062, and the value reaches the request.
func round(x float64, places int) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	v, _ := strconv.ParseFloat(strconv.FormatFloat(x, 'f', places, 64), 64)
	return v
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// toFloat reads a number the way Python's float() would, from anything JSON can produce.
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

// floatOr reads a number, falling back when the value is missing or not a number.
func floatOr(v any, fallback float64) float64 {
	if f, ok := toFloat(v); ok {
		return f
	}
	return fallback
}

func intOr(v any, fallback int) int {
	if f, ok := toFloat(v); ok {
		return int(f)
	}
	return fallback
}

// pyStr formats a value the way Python's str() would, so rule comparisons agree across languages.
func pyStr(v any) string {
	switch x := v.(type) {
	case nil:
		return "None"
	case string:
		return x
	case bool:
		if x {
			return "True"
		}
		return "False"
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e16 {
			return strconv.FormatFloat(x, 'f', 1, 64)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case int:
		return strconv.Itoa(x)
	}
	return fmt.Sprint(v)
}
