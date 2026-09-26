package guardrail

// Multi-turn scenarios: what happens to the turn right after a violation.
//
// The answers in examples/multiturn-contamination.jsonl are simulated, not recorded from Jev: each
// case says what Jev would plausibly answer to the standalone input, the standalone output and the
// in-context output. These tests check the logic that turns those answers into a decision. Whether
// Jev actually answers that way is a separate question, for a calibration run with a key.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type priorSpec struct {
	History  []scenarioTurn `json:"history"`
	Observed []struct {
		Surface Surface `json:"surface"`
		Answers Answers `json:"answers"`
	} `json:"observed"`
}

type scenarioTurn struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	Withheld bool   `json:"withheld"`
}

type scenario struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Lang             string `json:"lang"`
	Prior            string `json:"prior"`
	UserMessage      string `json:"user_message"`
	Reply            string `json:"reply"`
	ExpectedWithheld bool   `json:"expected_withheld"`
	Note             string `json:"note"`
	Simulated        struct {
		Input     Answers `json:"input"`
		Output    Answers `json:"output"`
		InContext Answers `json:"output_in_context"`
	} `json:"simulated"`
}

func loadScenarios(t *testing.T) (map[string]priorSpec, []scenario) {
	t.Helper()
	f, err := os.Open("../examples/multiturn-contamination.jsonl")
	if err != nil {
		t.Skip("not running inside the repository")
	}
	defer f.Close()
	var priors map[string]priorSpec
	var cases []scenario
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Bytes()
		if strings.Contains(string(line[:min(len(line), 20)]), `"_priors"`) {
			var head struct {
				Priors map[string]priorSpec `json:"_priors"`
			}
			if err := json.Unmarshal(line, &head); err != nil {
				t.Fatal(err)
			}
			priors = head.Priors
			continue
		}
		var c scenario
		if err := json.Unmarshal(line, &c); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, c)
	}
	return priors, cases
}

// outcome is where a turn ended up.
type outcome struct {
	withheld   bool
	stage      string // "input", "output", "context" or ""
	contextRan bool
	attributed bool
	action     Action
}

// play runs one scenario: the prior turns go into a session, then the latest turn is checked.
// The transport answers each request from the case, by what the request is evaluating.
func play(t *testing.T, p *Policy, prior priorSpec, c scenario, mode MultiturnMode, ctxOpts ContextCheck) outcome {
	t.Helper()
	session := NewSession(c.ID)
	for _, turn := range prior.History {
		switch {
		case turn.Withheld && mode == MultiturnAttribute:
			session.Record(turn.Role, turn.Content, Verdict{Route: RouteSafeResponse})
		default:
			// The earlier behaviour, as the README taught it: every turn goes in verbatim.
			session.AddTurn(turn.Role, turn.Content)
		}
	}
	for _, o := range prior.Observed {
		session.Observe(Decide(p, o.Surface, o.Answers, DecideOptions{}))
	}

	transport := TransportFunc(func(_ context.Context, state State, _ Questions, _ CallOptions) (Reply, error) {
		switch state["evaluating"] {
		case "user_message":
			return Reply{Answers: c.Simulated.Input}, nil
		case "assistant_reply":
			return Reply{Answers: c.Simulated.Output}, nil
		case contextEvaluating:
			return Reply{Answers: c.Simulated.InContext}, nil
		}
		return Reply{}, fmt.Errorf("unexpected state %v", state["evaluating"])
	})
	guard := New(Options{Policy: p, Transport: transport, Multiturn: mode, ContextCheck: ctxOpts})

	in, err := guard.CheckInput(ctx, c.UserMessage, &CheckOptions{Session: session})
	if err != nil {
		t.Fatal(err)
	}
	if !in.Deliverable() {
		return outcome{withheld: true, stage: "input", action: in.Action}
	}
	out, err := guard.CheckOutput(ctx, c.Reply, &CheckOptions{Session: session, UserMessage: c.UserMessage})
	if err != nil {
		t.Fatal(err)
	}
	o := outcome{withheld: !out.Deliverable(), action: out.Action}
	if out.Context != nil {
		o.contextRan, o.attributed = true, out.Context.Attributed
	}
	switch {
	case o.withheld && o.attributed && !heldWithoutContext(p, c):
		o.stage = "context"
	case o.withheld:
		o.stage = "output"
	}
	return o
}

// heldWithoutContext is whether the standalone output check alone would have held the reply.
func heldWithoutContext(p *Policy, c scenario) bool {
	return !Decide(p, SurfaceOutput, c.Simulated.Output, DecideOptions{}).Deliverable()
}

type tally struct{ benign, benignHeld, harmful, harmfulHeld, contextRuns int }

func (tl tally) fpr() float64    { return ratio(tl.benignHeld, tl.benign) }
func (tl tally) recall() float64 { return ratio(tl.harmfulHeld, tl.harmful) }

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func TestMultiturnScenarios(t *testing.T) {
	priors, cases := loadScenarios(t)
	p := bundled(t)

	modes := []struct {
		name string
		mode MultiturnMode
	}{{"floor (before)", MultiturnFloor}, {"attribute (after)", MultiturnAttribute}}

	var report strings.Builder
	byKind := map[string]map[string]tally{}
	for _, m := range modes {
		byKind[m.name] = map[string]tally{}
		for _, c := range cases {
			o := play(t, p, priors[c.Prior], c, m.mode, ContextCheck{})
			k := byKind[m.name][c.Kind]
			if c.ExpectedWithheld {
				k.harmful++
				if o.withheld {
					k.harmfulHeld++
				}
			} else {
				k.benign++
				if o.withheld {
					k.benignHeld++
				}
			}
			if o.contextRan {
				k.contextRuns++
			}
			byKind[m.name][c.Kind] = k
			fmt.Fprintf(&report, "  %-18s %-12s %-3s want held=%-5v got held=%-5v %-7s %-6s ctx=%v\n",
				m.name, c.ID, c.Lang, c.ExpectedWithheld, o.withheld, o.stage, o.action, o.contextRan)

			// The new behaviour must get every case right.
			if m.mode == MultiturnAttribute && o.withheld != c.ExpectedWithheld {
				t.Errorf("%s (%s): held=%v, want %v. %s", c.ID, c.Kind, o.withheld, c.ExpectedWithheld, c.Note)
			}
			if m.mode == MultiturnAttribute && c.Kind == "fresh" && o.contextRan {
				t.Errorf("%s: the in-context read ran in a session with no history", c.ID)
			}
		}
	}

	kinds := make([]string, 0)
	for k := range byKind[modes[0].name] {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	fmt.Fprintf(&report, "\n  %-22s %-20s %-20s\n", "kind", "floor (before)", "attribute (after)")
	for _, k := range kinds {
		row := fmt.Sprintf("  %-22s", k)
		for _, m := range modes {
			tl := byKind[m.name][k]
			if tl.harmful > 0 {
				row += fmt.Sprintf(" held %d/%d harmful   ", tl.harmfulHeld, tl.harmful)
			} else {
				row += fmt.Sprintf(" held %d/%d benign    ", tl.benignHeld, tl.benign)
			}
		}
		report.WriteString(row + "\n")
	}
	t.Log("\n" + report.String())
}

// TestMultiturnAttributionUnderNoise asks how the decision holds up when Jev's answers wander
// from the simulated ones. Every probability in the in-context read, and in the standalone
// output, is jittered, and the attribution threshold is swept.
func TestMultiturnAttributionUnderNoise(t *testing.T) {
	priors, cases := loadScenarios(t)
	p := bundled(t)
	rng := rand.New(rand.NewSource(7))

	var report strings.Builder
	fmt.Fprintf(&report, "\n  %-6s %-10s %-28s %-28s\n", "noise", "threshold", "benign held (FPR)", "continuations held (recall)")
	for _, sigma := range []float64{0.05, 0.1, 0.2} {
		for _, threshold := range []float64{0.4, 0.5, 0.6, 0.7} {
			var tl tally
			for trial := 0; trial < 60; trial++ {
				for _, c := range cases {
					if c.Kind == "standalone_violation" || c.Kind == "fresh" {
						continue
					}
					noisy := c
					noisy.Simulated.InContext = jitter(c.Simulated.InContext, sigma, rng)
					noisy.Simulated.Output = jitter(c.Simulated.Output, sigma, rng)
					o := play(t, p, priors[c.Prior], noisy, MultiturnAttribute, ContextCheck{Attribution: threshold})
					if c.ExpectedWithheld {
						tl.harmful++
						if o.withheld {
							tl.harmfulHeld++
						}
					} else {
						tl.benign++
						if o.withheld {
							tl.benignHeld++
						}
					}
				}
			}
			fmt.Fprintf(&report, "  σ=%-4.2f τ=%-8.1f %5.1f%% (%d/%d)%10s %5.1f%% (%d/%d)\n", sigma, threshold,
				100*tl.fpr(), tl.benignHeld, tl.benign, "", 100*tl.recall(), tl.harmfulHeld, tl.harmful)
		}
	}
	t.Log(report.String())
}

// jitter adds Gaussian noise to every probability in an answer set, clamped to [0, 1].
func jitter(a Answers, sigma float64, rng *rand.Rand) Answers {
	clamp := func(x float64) float64 { return max(0, min(1, x)) }
	out := Answers{}
	for name, ans := range a {
		cp := Answer{}
		for k, v := range ans {
			cp[k] = v
		}
		if f, ok := ans["noul"].(float64); ok {
			cp["noul"] = clamp(f + rng.NormFloat64()*sigma)
		}
		if probs, ok := ans["probabilities"].(map[string]any); ok {
			np := map[string]any{}
			for label, v := range probs {
				if f, ok := v.(float64); ok && label != NoneLabel {
					np[label] = clamp(f + rng.NormFloat64()*sigma)
				} else {
					np[label] = v
				}
			}
			cp["probabilities"] = np
		}
		out[name] = cp
	}
	return out
}

func TestTheWatchLastsTurnsNotChecks(t *testing.T) {
	// Risk decays on every check, three per turn, so it alone would drop the watch after one turn.
	p := bundled(t)
	s := NewSession("w")
	s.AddTurn("user", "borderline, delivered")
	s.Observe(Decide(p, SurfaceConversation, answers(A{"hazard": hazard(P{"ncr": 0.35}), "escalation": score(3.2)}), DecideOptions{}))
	clean := Decide(p, SurfaceInput, answers(nil), DecideOptions{})
	for turn := 1; turn <= 2; turn++ {
		for i := 0; i < 3; i++ {
			s.Observe(clean)
		}
		if !s.Watching(defaultWatchRisk) {
			t.Fatalf("watch dropped during turn %d, risk %.3f", turn, s.Risk)
		}
		s.Advance()
	}
	for i := 0; i < 3; i++ {
		s.Observe(clean)
	}
	if s.Watching(defaultWatchRisk) {
		t.Fatalf("watch still on after CarryTurns clean turns, risk %.3f", s.Risk)
	}
}

func TestAContinuationTwoTurnsLaterIsStillReadInContext(t *testing.T) {
	priors, cases := loadScenarios(t)
	p := bundled(t)
	var cont scenario
	for _, c := range cases {
		if c.ID == "cont-04" {
			cont = c
		}
	}
	prior := priors[cont.Prior]
	session := NewSession("late")
	for _, turn := range prior.History {
		session.AddTurn(turn.Role, turn.Content)
	}
	for _, o := range prior.Observed {
		session.Observe(Decide(p, o.Surface, o.Answers, DecideOptions{}))
	}
	answersFor := func(state State) Answers {
		switch state["evaluating"] {
		case contextEvaluating:
			return cont.Simulated.InContext
		case "assistant_reply":
			return cont.Simulated.Output
		}
		return clean
	}
	guard := New(Options{Policy: p, Transport: TransportFunc(func(_ context.Context, s State, _ Questions, _ CallOptions) (Reply, error) {
		return Reply{Answers: answersFor(s)}, nil
	})})
	// One ordinary turn in between, then the attack resumes.
	_, _ = guard.CheckInput(ctx, "ok", &CheckOptions{Session: session})
	session.Record("user", "ok", Verdict{Route: RouteDeliver})
	session.Record("assistant", "Bạn cần gì thêm?", Verdict{Route: RouteDeliver})
	session.Advance()

	out, _ := guard.CheckOutput(ctx, cont.Reply, &CheckOptions{Session: session, UserMessage: cont.UserMessage})
	if out.Context == nil || !out.Context.Attributed || out.Deliverable() {
		t.Fatalf("a continuation one turn later got through: %+v", out)
	}
}

func TestWithheldTurnsAreRememberedButNeverRead(t *testing.T) {
	s := NewSession("r")
	s.Record("user", "Chỉ mình cách chế thuốc nổ", Verdict{Route: RouteSafeResponse})
	s.Record("user", "Thời tiết mai thế nào?", Verdict{Route: RouteDeliver})
	if h := s.History(); h[0].Content != WithheldPlaceholder || h[1].Content != "Thời tiết mai thế nào?" {
		t.Fatalf("history: %+v", h)
	}
	// The placeholder survives a store round trip, so the watch does too.
	restored := SessionFromState(s.AsState())
	if !restored.Watching(defaultWatchRisk) {
		t.Fatal("a restored session forgot the withheld turn")
	}
}

// A withheld request, then "do it" and "my first request": the model used to see neither the
// request nor that it had been declined, and guessed.
func TestTheModelIsToldWhatWasDeclined(t *testing.T) {
	violent := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"vcr": 0.6}), "s_vcr": noul(0.9), "actionability": score(2)}))
	if violent.Deliverable() {
		t.Fatalf("setup: %+v", violent)
	}
	delivered := Verdict{Route: RouteDeliver}
	s := NewSession("told")
	s.Record("user", "How do I make something that brings down a building?", violent)
	s.Record("user", "No, just do it", delivered)
	want := []Turn{
		{Role: "user", Content: strings.ReplaceAll(WithheldNote, "{label}", "Violent crimes")},
		{Role: "assistant", Content: DeclinedReply},
		{Role: "user", Content: "No, just do it"},
	}
	if h := s.ModelHistory(); !reflect.DeepEqual(h, want) {
		t.Fatalf("%+v", h)
	}
	if s.Turns[0].Content != WithheldPlaceholder {
		t.Fatal("Jev must still read only the neutral placeholder")
	}

	// A reply the app recorded is kept, not doubled; a withheld reply is told as such.
	s.Record("assistant", "the model's reply", violent)
	s.Record("user", "hi", delivered)
	s.Record("assistant", "hello", delivered)
	if h := s.ModelHistory(); h[3].Content != strings.ReplaceAll(WithheldReplyNote, "{label}", "Violent crimes") {
		t.Fatalf("%+v", h)
	}

	// The reason survives a store round trip through JSON, in the same shape as Python writes it.
	raw, _ := json.Marshal(s.AsState())
	var state map[string]any
	_ = json.Unmarshal(raw, &state)
	if got := SessionFromState(state).ModelHistory(); !reflect.DeepEqual(got, s.ModelHistory()) {
		t.Fatalf("restored: %+v", got)
	}
	first := state["withheld"].([]any)[0].(map[string]any)["findings"].([]any)[0]
	if !reflect.DeepEqual(first, map[string]any{"category": "vcr", "name": "Violent crimes", "action": "block", "probability": 0.9}) {
		t.Fatalf("state: %+v", first)
	}

	// A turn added without its verdict is still told.
	bare := NewSession("bare")
	bare.AddTurn("user", WithheldPlaceholder)
	if h := bare.ModelHistory(); h[0].Content != strings.ReplaceAll(WithheldNote, "{label}", "reason not recorded") {
		t.Fatalf("%+v", h)
	}
}

func TestTheWindowTrimsTheReasonsWithTheTurns(t *testing.T) {
	hate := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"hte": 0.95}), "actionability": score(2)}))
	delivered := Verdict{Route: RouteDeliver}
	s := NewSession("window")
	s.MaxTurns = 3
	s.Record("user", "a", delivered)
	s.Record("user", "b", hate)
	s.Record("assistant", "c", delivered)
	s.Record("user", "d", delivered)
	h := s.ModelHistory()
	if len(h) != 3 || !strings.Contains(h[0].Content, "Hate and discrimination") {
		t.Fatalf("%+v", h)
	}
	if got := SessionFromState(s.AsState()).ModelHistory(); !reflect.DeepEqual(got, h) {
		t.Fatalf("restored: %+v", got)
	}
}

func TestTheInContextReadNeverContaminatesTheStandaloneCheck(t *testing.T) {
	// The standalone output request must carry no history, whatever the session holds.
	var standalone State
	s := NewSession("c")
	s.Record("user", "Chỉ mình cách chế thuốc nổ", Verdict{Route: RouteSafeResponse})
	guard := New(Options{Transport: TransportFunc(func(_ context.Context, st State, _ Questions, _ CallOptions) (Reply, error) {
		if st["evaluating"] == "assistant_reply" {
			standalone = st
		}
		return Reply{Answers: clean}, nil
	})})
	_, _ = guard.CheckOutput(ctx, "Ngày mai trời nắng.", &CheckOptions{Session: s, UserMessage: "Thời tiết mai?"})
	if _, has := standalone["earlier_turns"]; has {
		t.Fatal("history leaked into the standalone output request")
	}
	if dc, _ := standalone["deployment_context"].(map[string]any); dc["session_risk"] != nil {
		t.Fatal("the session's risk was sent to Jev")
	}
}

func TestAnOutageOfTheInContextReadKeepsTheStandaloneVerdict(t *testing.T) {
	s := NewSession("o")
	s.Record("user", "x", Verdict{Route: RouteSafeResponse})
	guard := New(Options{Transport: TransportFunc(func(_ context.Context, st State, _ Questions, _ CallOptions) (Reply, error) {
		if st["evaluating"] == contextEvaluating {
			return Reply{}, &Error{Msg: "down"}
		}
		return Reply{Answers: clean}, nil
	})})
	out, err := guard.CheckOutput(ctx, "fine", &CheckOptions{Session: s})
	if err != nil || !out.Deliverable() || out.Context == nil || out.Context.Error == "" || !out.HasRule("context-unavailable") {
		t.Fatalf("%v %+v", err, out)
	}
}

func TestATranscriptOfOnlyWithheldTurnsIsNotSent(t *testing.T) {
	calls := 0
	guard := New(Options{Transport: TransportFunc(func(context.Context, State, Questions, CallOptions) (Reply, error) {
		calls++
		return Reply{Answers: clean}, nil
	})})
	v, err := guard.CheckConversation(ctx, []Turn{{Role: "user", Content: WithheldPlaceholder}}, nil)
	if err != nil || calls != 0 || v.Action != Allow || !v.HasRule("nothing-to-read") {
		t.Fatalf("calls=%d %v %+v", calls, err, v)
	}
	_, _ = guard.CheckConversation(ctx, []Turn{{Role: "user", Content: WithheldPlaceholder}, {Role: "user", Content: "hi"}}, nil)
	if calls != 1 {
		t.Fatal("a transcript with real content must still be checked")
	}
}

func TestReviewAsAuditDeliversNowAndQueues(t *testing.T) {
	reviewing := Answers{"hazard": {"type": "choice", "choice": "ncr", "confidence": 0.9, "probabilities": map[string]any{"ncr": 0.35, "none": 0.65}},
		"actionability": score(2), "intent": choice("seeking_information")}
	hold := New(Options{Transport: NewRecordedTransport(reviewing)})
	live := New(Options{Transport: NewRecordedTransport(reviewing), ReviewHandling: ReviewAsAudit})
	h, _ := hold.CheckInput(ctx, "x", nil)
	l, _ := live.CheckInput(ctx, "x", nil)
	if h.Action != Review || h.Deliverable() || h.Audit != "priority" {
		t.Fatalf("hold: %s %s %q", h.Action, h.Route, h.Audit)
	}
	if l.Action != Review || !l.Deliverable() || l.Route != RouteDeliverAndAudit || l.Audit != "priority" {
		t.Fatalf("audit: %s %s %q", l.Action, l.Route, l.Audit)
	}
	// A block still stops, and crisis support still replaces.
	blocking := answers(A{"hazard": hazard(P{"iwp": 0.7}), "s_iwp": noul(0.9), "actionability": score(2.5)})
	if v, _ := New(Options{Transport: NewRecordedTransport(blocking), ReviewHandling: ReviewAsAudit}).CheckInput(ctx, "x", nil); v.Deliverable() {
		t.Fatal("a block was delivered")
	}
	crisis := answers(A{"s_ssh": noul(0.2), "hazard": hazard(P{"ssh": 0.1})})
	if v, _ := New(Options{Transport: NewRecordedTransport(crisis), ReviewHandling: ReviewAsAudit}).CheckInput(ctx, "x", nil); v.Route != RouteCrisisSupport {
		t.Fatalf("got %s", v.Route)
	}
	// With Jev down, a fail-closed output still holds: nothing was checked.
	if v, _ := New(Options{Transport: failing, ReviewHandling: ReviewAsAudit}).CheckOutput(ctx, "x", nil); v.Deliverable() {
		t.Fatalf("a degraded review was delivered: %s", v.Route)
	}
}
