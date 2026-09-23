package guardrail

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func criteriaLabels(t *testing.T, q Question) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(q.Criteria))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var labels []string
	for dec.More() {
		key, _ := dec.Token()
		labels = append(labels, key.(string))
		var skip any
		_ = dec.Decode(&skip)
	}
	return labels
}

func TestEverySurfaceBuildsAValidQuestionSet(t *testing.T) {
	p := bundled(t)
	for _, surface := range Surfaces {
		questions, err := BuildQuestions(p, surface, false, "")
		if err != nil {
			t.Fatal(err)
		}
		labels := criteriaLabels(t, questions[Hazard])
		if labels[0] != NoneLabel {
			t.Fatalf("%s: none must come first, got %v", surface, labels)
		}
		if len(labels) > 256 {
			t.Fatalf("choice questions take at most 255 labels")
		}
		for name, q := range questions {
			if q.Type != "noul" && q.Type != "choice" && q.Type != "score" {
				t.Fatalf("%s: type %q", name, q.Type)
			}
			if q.Instructions == "" {
				t.Fatalf("%s: no instructions", name)
			}
			if q.Type == "score" {
				var criteria []any
				if err := json.Unmarshal(q.Criteria, &criteria); err != nil || len(criteria) < 2 || len(criteria) > 10 {
					t.Fatalf("%s: score criteria %s", name, q.Criteria)
				}
			}
			if q.Type == "choice" && len(q.Criteria) == 0 {
				t.Fatalf("%s: choice without criteria", name)
			}
		}
	}
}

func TestHazardLabelsMatchTheSurface(t *testing.T) {
	questions, _ := BuildQuestions(bundled(t), SurfaceInput, false, "")
	labels := criteriaLabels(t, questions[Hazard])
	if contains(labels, "mis") {
		t.Fatal("output-only category leaked into the input question")
	}
	if !contains(labels, "pij") || contains(labels, "ipv") {
		t.Fatalf("got %v", labels)
	}
}

func TestHazardLabelsGoMostSeriousFirst(t *testing.T) {
	p := bundled(t)
	questions, _ := BuildQuestions(p, SurfaceInput, false, "")
	var want []string
	for _, c := range p.ForSurface(SurfaceInput) {
		want = append(want, c.ID)
	}
	if got := criteriaLabels(t, questions[Hazard])[1:]; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestScaleCriteriaKeepTheirOrder(t *testing.T) {
	questions, _ := BuildQuestions(bundled(t), SurfaceInput, false, "")
	labels := criteriaLabels(t, questions["intent"])
	if labels[0] != "benign" || labels[1] != "academic" {
		t.Fatalf("intent labels reordered: %v", labels)
	}
}

func TestGroundednessAppearsOnlyWithContext(t *testing.T) {
	p := bundled(t)
	without, _ := BuildQuestions(p, SurfaceOutput, false, "")
	with, _ := BuildQuestions(p, SurfaceOutput, true, "")
	if _, ok := without["groundedness"]; ok {
		t.Fatal("groundedness asked without context")
	}
	if _, ok := with["groundedness"]; !ok {
		t.Fatal("groundedness missing with context")
	}
}

func TestDisabledCategoriesAreLeftOut(t *testing.T) {
	questions, _ := BuildQuestions(bundled(t), SurfaceInput, false, "")
	if contains(criteriaLabels(t, questions[Hazard]), "scp") {
		t.Fatal("disabled category asked")
	}
}

func TestPolicyCopiesStayInSync(t *testing.T) {
	canonical, err := os.ReadFile("../policies/standard-v1.json")
	if err != nil {
		t.Skip("not running inside the repository")
	}
	shipped, _ := os.ReadFile("policies/standard-v1.json")
	var a, b any
	_ = json.Unmarshal(canonical, &a)
	_ = json.Unmarshal(shipped, &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("go/policies/standard-v1.json drifted; run scripts/sync-policies.sh")
	}
}

func TestBundledPackHasEighteenCategories(t *testing.T) {
	if n := len(bundled(t).Categories); n != 18 {
		t.Fatalf("got %d", n)
	}
}

func TestBadThresholdsAreRejected(t *testing.T) {
	_, err := NewPolicy(map[string]any{
		"id":         "broken",
		"categories": map[string]any{"x": map[string]any{"thresholds": map[string]any{"default": map[string]any{"block": 0.1, "review": 0.5, "flag": 0.2}}}},
	})
	if err == nil || !strings.Contains(err.Error(), "not ordered") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadPolicyByNameAndPath(t *testing.T) {
	byName, err := LoadPolicy("standard-v1")
	if err != nil {
		t.Fatal(err)
	}
	byPath, err := LoadPolicy("policies/standard-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if byName.QualifiedID() != byPath.QualifiedID() {
		t.Fatal("same pack loaded two ways disagrees")
	}
	if _, err := LoadPolicy("no-such-pack"); err == nil {
		t.Fatal("unknown pack loaded")
	}
}

func TestGuardSendsOneRequestPerCheck(t *testing.T) {
	transport := NewRecordedTransport(clean)
	guard := New(Options{Transport: transport})
	v, err := guard.CheckInput(context.Background(), "hello there", nil)
	if err != nil || v.Action != Allow {
		t.Fatalf("%v %+v", err, v)
	}
	calls := transport.Calls()
	if len(calls) != 1 || calls[0].State["user_message"] != "hello there" {
		t.Fatalf("got %+v", calls)
	}
	if _, ok := calls[0].Questions[Hazard]; !ok {
		t.Fatal("no hazard question")
	}
}

func TestPreviewNeedsNoAPIKey(t *testing.T) {
	t.Setenv("JEV_API_KEY", "")
	preview, err := New(Options{}).Preview(SurfaceInput, State{"user_message": "hi"}, false, "")
	if err != nil || preview.Questions[Hazard].Type != "choice" {
		t.Fatalf("%v %+v", err, preview)
	}
}
