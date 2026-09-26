package guardrail

import (
	"encoding/json"
	"strings"
	"testing"
)

func decideOn(p *Policy, surface Surface, a Answers) Verdict {
	return Decide(p, surface, a, DecideOptions{})
}

func TestCleanInputIsAllowed(t *testing.T) {
	v := decideOn(bundled(t), SurfaceInput, answers(nil))
	if v.Action != Allow || !v.Allowed() || len(v.Findings) != 0 || v.Route != RouteDeliver {
		t.Fatalf("got %+v", v)
	}
}

func TestSentinelOverridesADilutedChoice(t *testing.T) {
	// A choice spreads its mass across 18 labels; the sentinel is the one that must not miss.
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"cse": 0.09}), "s_cse": noul(0.82)}))
	if v.Action != Block || v.Top() == nil || v.Top().Category != "cse" || !approx(v.Top().Probability, 0.82) {
		t.Fatalf("got %+v", v)
	}
}

func TestNeverBelowForcesBlockOnAWeakSignal(t *testing.T) {
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"cse": 0.04})}))
	if v.Action != Block || !contains(v.Top().Notes, "never below block") {
		t.Fatalf("got %+v", v)
	}
}

func TestAcademicIntentSoftensButNotForWeapons(t *testing.T) {
	p := bundled(t)
	weapons := answers(A{"hazard": hazard(P{"iwp": 0.5}), "s_iwp": noul(0.5), "intent": choice("academic"), "actionability": score(2)})
	if got := decideOn(p, SurfaceInput, weapons).Action; got != Block {
		t.Fatalf("weapons: %s", got)
	}
	fraud := answers(A{"hazard": hazard(P{"ncr": 0.55}), "intent": choice("academic"), "actionability": score(2)})
	v := decideOn(p, SurfaceInput, fraud)
	if v.Action != Review || !v.HasRule("academic-intent-softens") {
		t.Fatalf("fraud: %+v", v)
	}
}

func TestActionabilityMovesTheVerdictBothWays(t *testing.T) {
	p := bundled(t)
	base := func(actionability float64) Answers {
		return Answers{"hazard": hazard(P{"vcr": 0.3}), "severity": score(2), "intent": choice("seeking_information"), "actionability": score(actionability)}
	}
	talk := decideOn(p, SurfaceInput, base(0.5))
	if talk.Action != Flag || talk.Route != RouteDeliver || !talk.HasRule("low-actionability-softens") {
		t.Fatalf("talk: %+v", talk)
	}
	recipe := decideOn(p, SurfaceInput, base(3))
	if recipe.Action != Block || !recipe.HasRule("high-actionability-hardens") {
		t.Fatalf("recipe: %+v", recipe)
	}
}

func TestEvasionRaisesAPromptInjectionFinding(t *testing.T) {
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"ncr": 0.2}), "intent": choice("evasion")}))
	if !v.HasCategory("pij") || !v.HasRule("evasion-hardens") {
		t.Fatalf("got %+v", v)
	}
}

func TestSelfHarmRoutesToCrisisSupport(t *testing.T) {
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"ssh": 0.2}), "s_ssh": noul(0.66)}))
	if v.Action != Block || v.Route != RouteCrisisSupport || v.Deliverable() {
		t.Fatalf("got %+v", v)
	}
}

func TestPIIInAReplyIsRedactedNotBlocked(t *testing.T) {
	v := decideOn(bundled(t), SurfaceOutput, answers(A{"hazard": hazard(P{"prv": 0.3}), "s_prv": noul(0.35), "refusal": noul(0.02)}))
	if v.Action != Review || v.Route != RouteRedact || !v.Deliverable() {
		t.Fatalf("got %+v", v)
	}
}

func TestARefusalIsNotPunishedForNamingTheHazard(t *testing.T) {
	v := decideOn(bundled(t), SurfaceOutput, answers(A{"hazard": hazard(P{"vcr": 0.62}), "refusal": noul(0.95), "actionability": score(0)}))
	if v.Action != Flag || !v.HasRule("refusal-caps-output-risk") {
		t.Fatalf("got %+v", v)
	}
}

func TestARefusalDoesNotExcuseALeakedSystemPrompt(t *testing.T) {
	v := decideOn(bundled(t), SurfaceOutput, answers(A{"hazard": hazard(P{"sid": 0.5}), "s_sid": noul(0.7), "refusal": noul(0.95)}))
	if v.Action != Block {
		t.Fatalf("got %+v", v)
	}
}

func TestUngroundedReplyRaisesMisinformation(t *testing.T) {
	v := decideOn(bundled(t), SurfaceOutput, answers(A{"groundedness": score(0.5), "refusal": noul(0)}))
	if v.Action != Flag || !v.HasCategory("mis") || !v.HasRule("ungrounded-output") {
		t.Fatalf("got %+v", v)
	}
}

func TestGroundedReplyStaysClean(t *testing.T) {
	v := decideOn(bundled(t), SurfaceOutput, answers(A{"groundedness": score(3), "refusal": noul(0)}))
	if v.Action != Allow {
		t.Fatalf("got %+v", v)
	}
}

func TestCrescendoEscalatesAConversation(t *testing.T) {
	v := decideOn(bundled(t), SurfaceConversation, answers(A{"hazard": hazard(P{"ncr": 0.32}), "crescendo": noul(0.8), "escalation": score(3.4)}))
	if (v.Action != Review && v.Action != Block) || !v.HasCategory("pij") ||
		!v.HasRule("crescendo-hardens") || !v.HasRule("escalation-review") {
		t.Fatalf("got %+v", v)
	}
}

func TestLowConfidenceEscalatesToReview(t *testing.T) {
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"ncr": 0.32}, 0.2)}))
	if v.Action != Review || !v.HasRule("confidence-gate") || v.Allowed() {
		t.Fatalf("got %+v", v)
	}
}

func TestLowConfidenceWithNothingNearbyStaysAllowed(t *testing.T) {
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"ncr": 0.01}, 0.2)}))
	if v.Action != Allow {
		t.Fatalf("got %+v", v)
	}
}

func TestLowConfidenceNeverDowngradesABlock(t *testing.T) {
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"vcr": 0.9}, 0.1), "actionability": score(2)}))
	if v.Action != Block || v.Route != RouteSafeResponse {
		t.Fatalf("got %+v", v)
	}
}

func TestInputDropsOutputOnlyCategories(t *testing.T) {
	// mis is an output-only category and must not be scored on a user message.
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"mis": 0.9})}))
	if v.HasCategory("mis") {
		t.Fatalf("got %+v", v)
	}
}

func TestErrorVerdictFailsClosedOnOutput(t *testing.T) {
	// The output check is the last line, so an unreachable Jev holds the reply.
	v := ErrorVerdict(bundled(t), SurfaceOutput, &Error{Msg: "boom"}, 0)
	if v.Action != Review || v.Route != RouteHumanReview {
		t.Fatalf("got %+v", v)
	}
	if v.Deliverable() {
		t.Fatal("a degraded verdict must not read as permission to send")
	}
	if !v.Degraded || v.Allowed() || !strings.Contains(v.Error, "boom") {
		t.Fatalf("got %+v", v)
	}
}

func TestVerdictSerialises(t *testing.T) {
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"hte": 0.6}), "actionability": score(2)}))
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Action   string `json:"action"`
		PolicyID string `json:"policy_id"`
		Findings []struct {
			Category string `json:"category"`
		} `json:"findings"`
		Prefilter *string `json:"prefilter"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Action != "block" || payload.Findings[0].Category != "hte" ||
		!strings.HasPrefix(payload.PolicyID, "standard-v1@") || payload.Prefilter != nil {
		t.Fatalf("got %s", encoded)
	}
}

func TestASofteningRuleNeverErasesAFinding(t *testing.T) {
	// Softening lowers the response; the record of what fired stays.
	v := decideOn(bundled(t), SurfaceInput, answers(A{"hazard": hazard(P{"ncr": 0.16}), "actionability": score(0), "intent": choice("academic")}))
	if v.Action != Flag || !v.HasCategory("ncr") {
		t.Fatalf("got %+v", v)
	}
}

func TestActionAndRouteAreSeparateAxes(t *testing.T) {
	// A specialized-advice reply is delivered, but steered.
	v := decideOn(bundled(t), SurfaceOutput, answers(A{"hazard": hazard(P{"spc": 0.55}), "refusal": noul(0), "actionability": score(2)}))
	if v.Action != Review || v.Route != RouteGuide || !v.Deliverable() {
		t.Fatalf("got %+v", v)
	}
}

func TestASentinelProbabilityIsNotReadAsDoubt(t *testing.T) {
	// Treating a noul's distance from 0.5 as confidence double-counts the probability: every
	// mid-range sentinel would escalate to review no matter where the threshold sat.
	v := decideOn(bundled(t), SurfaceOutput, answers(A{"hazard": hazard(P{"prv": 0.05}, 0.9), "s_prv": noul(0.2), "refusal": noul(0)}))
	if v.Action != Flag || v.HasRule("confidence-gate") || v.Top().Source != "sentinel" || !approx(v.Top().Confidence, 0.9) {
		t.Fatalf("got %+v", v)
	}
}

func TestASentinelStillInheritsAShakyRequest(t *testing.T) {
	v := decideOn(bundled(t), SurfaceOutput, answers(A{"hazard": hazard(P{"prv": 0.05}, 0.2), "s_prv": noul(0.2), "refusal": noul(0)}))
	if v.Action != Review || !v.HasRule("confidence-gate") {
		t.Fatalf("got %+v", v)
	}
}

func TestAnUncorroboratedSentinelAnswersAtItsOwnBand(t *testing.T) {
	p := bundled(t)
	// The choice backs it (0.04 is well above 0.01): never_below still lifts it to block.
	if v := decideOn(p, SurfaceInput, answers(A{"hazard": hazard(P{"cse": 0.04})})); v.Action != Block {
		t.Fatalf("corroborated: %s", v.Action)
	}
	// A sentinel on its own, the choice giving cse nothing.
	cases := []struct {
		sentinel float64
		want     Action
	}{{0.04, Flag}, {0.1, Review}, {0.2, Block}}
	for _, c := range cases {
		v := decideOn(p, SurfaceInput, answers(A{"s_cse": noul(c.sentinel)}))
		if v.Action != c.want || !v.Top().Uncorroborated {
			t.Fatalf("s_cse=%v: got %s, want %s (%+v)", c.sentinel, v.Action, c.want, v.Top())
		}
	}
}

func TestARefusalIsNotBlockedForAnUncorroboratedSentinel(t *testing.T) {
	p := bundled(t)
	refusing := answers(A{"s_cse": noul(0.23), "s_iwp": noul(0.14), "refusal": noul(0.9)})
	if v := decideOn(p, SurfaceOutput, refusing); v.Action != Flag || !v.Deliverable() {
		t.Fatalf("got %+v", v)
	}
	// A refusal can still leak a system prompt: sid is exempt from the cap.
	leaking := answers(A{"s_sid": noul(0.35), "refusal": noul(0.9)})
	if v := decideOn(p, SurfaceOutput, leaking); v.Deliverable() {
		t.Fatalf("a refusal leaking a system prompt got through: %+v", v)
	}
	// A strong sentinel is not capped, refusal or not.
	if v := decideOn(p, SurfaceOutput, answers(A{"s_cse": noul(0.6), "refusal": noul(0.9)})); v.Action != Block {
		t.Fatalf("got %s", v.Action)
	}
	// Without the pack setting, nothing changes.
	var pack map[string]any
	data, _ := bundledPacks.ReadFile("policies/standard-v1.json")
	_ = json.Unmarshal(data, &pack)
	delete(pack["defaults"].(map[string]any), "sentinel_corroboration")
	old, _ := NewPolicy(pack)
	if v := decideOn(old, SurfaceOutput, refusing); v.Action != Block {
		t.Fatalf("with the setting removed: %s", v.Action)
	}
}

func TestAWeakUncorroboratedSelfHarmSignalDoesNotReplaceTheAnswer(t *testing.T) {
	p := bundled(t)
	// "Còn ăn uống thì sao?" in a travel conversation, measured: s_ssh 0.07, choice ssh 0.01.
	v := decideOn(p, SurfaceInput, answers(A{"hazard": hazard(P{"ssh": 0.01}, 0.98), "s_ssh": noul(0.07)}))
	if v.Action != Flag || v.Route != RouteDeliver || !v.Deliverable() {
		t.Fatalf("got %s %s", v.Action, v.Route)
	}
	// A corroborated or strong signal still goes to crisis support.
	for _, a := range []Answers{
		answers(A{"hazard": hazard(P{"ssh": 0.2}), "s_ssh": noul(0.1)}),
		answers(A{"s_ssh": noul(0.4)}),
	} {
		if v := decideOn(p, SurfaceInput, a); v.Route != RouteCrisisSupport {
			t.Fatalf("got %s %s", v.Action, v.Route)
		}
	}
}
