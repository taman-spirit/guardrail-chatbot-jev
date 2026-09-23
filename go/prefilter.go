package guardrail

import (
	"fmt"
	"regexp"
	"strings"
)

// Prefilter is a deterministic check that runs before Jev, and short-circuits it.
//
// Jev reads content; it does not match patterns, count, or do arithmetic. A card number, a leaked
// key format, a banned term: these are decided exactly by a regex, in microseconds, without a
// network call. Putting them in front of the model saves a round trip on the obvious cases and
// keeps the part of the policy that has to be auditable out of a probability.
//
// Decide returns a verdict and true to settle the check, or false to let Jev decide.
type Prefilter interface {
	Decide(p *Policy, surface Surface, state State) (Verdict, bool)
}

// PrefilterFunc adapts a function to the Prefilter interface.
type PrefilterFunc func(p *Policy, surface Surface, state State) (Verdict, bool)

// Decide calls f.
func (f PrefilterFunc) Decide(p *Policy, surface Surface, state State) (Verdict, bool) {
	return f(p, surface, state)
}

// Pattern is one deterministic rule.
type Pattern struct {
	Name     string
	Regex    *regexp.Regexp
	Category string
	Action   Action
	// Surfaces defaults to every surface.
	Surfaces []Surface
}

// MustPattern compiles a pattern, panicking on a bad expression. Unlike the Python package,
// matching is case-sensitive unless the expression starts with (?i), and \b is ASCII-only: use
// the unicode-aware boundaries in CommonPatterns when the text around a match may be Vietnamese.
func MustPattern(name, expr, category string, action Action, surfaces ...Surface) Pattern {
	return Pattern{Name: name, Regex: regexp.MustCompile(expr), Category: category, Action: action, Surfaces: surfaces}
}

func (p Pattern) appliesTo(surface Surface) bool {
	if len(p.Surfaces) == 0 {
		return true
	}
	for _, s := range p.Surfaces {
		if s == surface {
			return true
		}
	}
	return false
}

// PatternPrefilter matches patterns against the text in a state and settles the check on the
// first hit. Order matters only in that the first match wins, so put the most specific first.
type PatternPrefilter struct {
	Patterns []Pattern
}

// Decide settles the check on the first matching pattern.
func (pf PatternPrefilter) Decide(p *Policy, surface Surface, state State) (Verdict, bool) {
	text := textOf(state)
	if text == "" {
		return Verdict{}, false
	}
	for _, pattern := range pf.Patterns {
		if !pattern.appliesTo(surface) || !pattern.Regex.MatchString(text) {
			continue
		}
		cat, ok := p.Categories[pattern.Category]
		if !ok || !cat.Applies(surface) {
			continue
		}
		action := pattern.Action
		if action == "" {
			action = Block
		}
		return PrefilterVerdict(p, surface, cat.ID, action, pattern.Name), true
	}
	return Verdict{}, false
}

// PrefilterVerdict builds a verdict that looks like any other, so callers need no special case.
func PrefilterVerdict(p *Policy, surface Surface, categoryID string, action Action, rule string) Verdict {
	cat := p.Categories[categoryID]
	r := RouteDeliver
	switch action {
	case Block:
		r = RouteSafeResponse
	case Review:
		r = RouteHumanReview
	}
	if (cat.Route == RouteRedact || cat.Route == RouteGuide) && action != Block {
		r = cat.Route
	} else if cat.Route == RouteCrisisSupport && action != Allow {
		r = RouteCrisisSupport
	}
	severity := float64(cat.BaseSeverity)
	return Verdict{
		Action:  action,
		Surface: surface,
		Route:   r,
		Findings: []Finding{{
			Category:    cat.ID,
			Name:        cat.Name,
			Probability: 1,
			Confidence:  1,
			Action:      action,
			Severity:    severity,
			Refs:        cat.Refs,
			Source:      "prefilter:" + rule,
			Notes:       []string{fmt.Sprintf("matched by %s, Jev was not called", rule)},
		}},
		Signals:    map[string]any{},
		Confidence: 1,
		Severity:   severity,
		PolicyID:   p.QualifiedID(),
		Prefilter:  rule,
	}
}

// CommonPatterns is a starting set. Every deployment should replace these with its own.
//
// These are examples of the shape, not a recommended list: what counts as a banned term is a
// policy question for the deployment, and a pattern that is wrong blocks real users silently.
var CommonPatterns = []Pattern{
	MustPattern("openai-style-key", wordStart+`sk-[A-Za-z0-9]{20,}`+wordEnd, "sid", Block, SurfaceOutput, SurfaceConversation),
	MustPattern("jev-api-key", wordStart+`apikey_[a-f0-9]{30,}`+wordEnd, "sid", Block, SurfaceOutput, SurfaceConversation),
	MustPattern("private-key-block", `-----BEGIN (RSA |EC |OPENSSH |PGP )?PRIVATE KEY-----`, "sid", Block, SurfaceOutput, SurfaceConversation),
	MustPattern("vn-national-id", wordStart+`0\d{11}`+wordEnd, "prv", Review, SurfaceInput, SurfaceOutput, SurfaceConversation),
}

// Go's \b counts only ASCII letters as word characters, so "số012345678901" has a boundary before
// the 0 in Go and none in Python. These count every letter and digit, as Python's \b does.
const (
	wordStart = `(?:^|[^\p{L}\p{N}_])`
	wordEnd   = `(?:$|[^\p{L}\p{N}_])`
)

// textOf pulls the checkable text out of any of the three state shapes.
func textOf(state State) string {
	if turns, ok := state["turns"]; ok {
		var parts []string
		switch ts := turns.(type) {
		case []Turn:
			for _, t := range ts {
				parts = append(parts, t.Content)
			}
		case []map[string]any:
			for _, t := range ts {
				parts = append(parts, turnFromMap(t).Content)
			}
		case []any:
			for _, t := range ts {
				if m, isMap := t.(map[string]any); isMap {
					parts = append(parts, turnFromMap(m).Content)
				}
			}
		default:
			return ""
		}
		return strings.Join(parts, "\n")
	}
	var parts []string
	for _, key := range []string{"user_message", "assistant_reply"} {
		if s, ok := state[key].(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}
