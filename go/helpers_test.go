package guardrail

import (
	"math"
	"testing"
)

func bundled(t *testing.T) *Policy {
	t.Helper()
	p, err := BundledPolicy("standard-v1")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// hazard is a choice answer over the hazard taxonomy.
func hazard(probabilities map[string]float64, confidence ...float64) Answer {
	c := 0.9
	if len(confidence) > 0 {
		c = confidence[0]
	}
	filled := map[string]any{}
	total := 0.0
	for k, v := range probabilities {
		filled[k] = v
		total += v
	}
	if _, ok := filled["none"]; !ok {
		filled["none"] = math.Max(0, 1-total)
	}
	top, best := "", -1.0
	for k, v := range filled {
		if f := v.(float64); f > best {
			top, best = k, f
		}
	}
	return Answer{"type": "choice", "choice": top, "confidence": c, "probabilities": filled}
}

func noul(v float64) Answer { return Answer{"type": "noul", "noul": v} }

func score(v float64) Answer {
	return Answer{"type": "score", "score": v, "confidence": 0.9, "probabilities": map[string]any{}, "legend": map[string]any{}}
}

func choice(v string) Answer {
	return Answer{"type": "choice", "choice": v, "confidence": 0.9, "probabilities": map[string]any{v: 0.9}}
}

// answers assembles an answer set, defaulting the always-present signals to neutral values.
func answers(parts map[string]Answer) Answers {
	base := Answers{
		"hazard":        hazard(nil),
		"severity":      score(0),
		"actionability": score(0),
		"intent":        choice("benign"),
	}
	for k, v := range parts {
		base[k] = v
	}
	return base
}

type A = map[string]Answer
type P = map[string]float64

var clean = Answers{"hazard": {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": map[string]any{"none": 0.95}}}

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func contains[T comparable](items []T, want T) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
