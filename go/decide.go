package guardrail

import (
	"fmt"
	"sort"
)

// Answer is one answer in Jev's wire shape, for example
// {"type": "noul", "noul": 0.82} or {"type": "choice", "choice": "none", "confidence": 0.9, ...}.
type Answer map[string]any

// Answers are keyed by question name.
type Answers map[string]Answer

// categoryRoutes are the handling modes a category may ask for when it fires below the block line.
var categoryRoutes = map[Route]bool{RouteRedact: true, RouteGuide: true, RouteCrisisSupport: true}

// DecideOptions carries the call details recorded on the verdict.
type DecideOptions struct {
	Model     string
	Usage     Usage
	LatencyMS float64
}

// Decide turns one set of Jev answers into a verdict.
//
// It makes no network calls and holds no state, so the whole policy can be tested against
// recorded answers without an API key.
func Decide(p *Policy, surface Surface, answers Answers, opts DecideOptions) Verdict {
	signals := readSignals(p, answers)
	probabilities, confidences, sentinelSourced := hazardProbabilities(p, surface, answers)

	corroboration, checkCorroboration := p.SentinelCorroboration()
	choice := mapOf(answers[Hazard]["probabilities"])

	var findings []Finding
	for _, cat := range p.ForSurface(surface) {
		confidence, ok := confidences[cat.ID]
		if !ok {
			confidence = 1.0
		}
		uncorroborated := checkCorroboration && sentinelSourced[cat.ID] &&
			floatOr(choice[cat.ID], 0) < corroboration.MinChoice
		if f, fired := finding(cat, surface, probabilities[cat.ID], confidence, sentinelSourced[cat.ID], uncorroborated); fired {
			findings = append(findings, f)
		}
	}

	findings, floors, applied := applyRules(p, surface, findings, signals, probabilities)
	if checkCorroboration && surface == SurfaceOutput {
		findings = capUncorroboratedOnRefusal(findings, signals, corroboration)
	}
	if checkCorroboration && corroboration.WeakAtMostFlag {
		for i, f := range findings {
			if f.weak && Rank(f.Action) > Rank(Flag) {
				f.Notes = append(append([]string(nil), f.Notes...), fmt.Sprintf("uncorroborated sentinel below its block band: %s -> flag", f.Action))
				f.Action = Flag
				findings[i] = f
			}
		}
	}

	action := Allow
	for _, f := range findings {
		action = Stronger(action, f.Action)
	}
	for _, floor := range floors {
		action = Stronger(action, floor)
	}

	confidence := overallConfidence(answers, findings, confidences)
	action, escalated := confidenceGate(p, action, confidence, findings, probabilities, surface, choice, signals, corroboration.MinChoice)
	if escalated {
		applied = append(applied, "confidence-gate")
	}

	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if Rank(a.Action) != Rank(b.Action) {
			return Rank(a.Action) > Rank(b.Action)
		}
		return a.Probability > b.Probability
	})

	return Verdict{
		Action:       action,
		Surface:      surface,
		Findings:     findings,
		Signals:      signals,
		Confidence:   confidence,
		Severity:     severity(signals, findings),
		Route:        route(p, findings, action),
		AppliedRules: applied,
		Model:        opts.Model,
		Usage:        opts.Usage,
		LatencyMS:    opts.LatencyMS,
		PolicyID:     p.QualifiedID(),
	}
}

// ErrorVerdict is the verdict to use when Jev could not be reached.
//
// Fail-closed is the default: an unavailable guardrail is not an approval.
func ErrorVerdict(p *Policy, surface Surface, err error, latencyMS float64) Verdict {
	action := Allow
	if p.FailClosed(surface) {
		action = p.ErrorAction()
	}
	// The route has to follow the action here too: a degraded "review" that still says "deliver"
	// reads as permission to send, which is the opposite of what failing closed means.
	r := RouteDeliver
	switch action {
	case Block:
		r = RouteSafeResponse
	case Review:
		r = RouteHumanReview
	}
	msg := "unknown error"
	if err != nil {
		msg = fmt.Sprintf("%s: %v", errorKind(err), err)
	}
	return Verdict{
		Action:     action,
		Surface:    surface,
		Route:      r,
		Confidence: 0,
		Degraded:   true,
		Error:      msg,
		LatencyMS:  latencyMS,
		PolicyID:   p.QualifiedID(),
	}
}

// WithFloor raises a verdict to at least floor, recomputing the route to match.
//
// Used to carry risk forward: a session that has already shown an escalation pattern should not
// have its next turn judged as if the conversation had just started.
func WithFloor(p *Policy, v Verdict, floor Action, note string) Verdict {
	action := Stronger(v.Action, floor)
	if action == v.Action {
		return v
	}
	v.Action = action
	v.Route = route(p, v.Findings, action)
	v.AppliedRules = append(append([]string(nil), v.AppliedRules...), note)
	return v
}

// capUncorroboratedOnRefusal lowers to flag an uncorroborated sentinel finding on a reply that
// declines. Measured against Jev, a plain refusal ("I can't help with that") drew sentinel answers of
// up to 0.26 for categories the reply never touched, while the hazard choice gave them nothing. The
// categories a refusal can still leak, such as a system prompt or personal data, are exempt.
func capUncorroboratedOnRefusal(findings []Finding, signals map[string]any, c SentinelCorroboration) []Finding {
	refusal, ok := signals["refusal"].(float64)
	if !ok || refusal < c.Refusal {
		return findings
	}
	for i, f := range findings {
		if !f.Uncorroborated || f.Probability >= c.RefusalMaxSentinel || c.RefusalExcept[f.Category] || Rank(f.Action) <= Rank(Flag) {
			continue
		}
		f.Notes = append(append([]string(nil), f.Notes...), fmt.Sprintf("refusal with an uncorroborated sentinel: %s -> flag", f.Action))
		f.Action = Flag
		findings[i] = f
	}
	return findings
}

// -- internals -------------------------------------------------------------

func errorKind(err error) string {
	if _, ok := err.(*Error); ok {
		return "GuardrailError"
	}
	return fmt.Sprintf("%T", err)
}

func readSignals(p *Policy, answers Answers) map[string]any {
	signals := map[string]any{}
	for name := range p.Signals {
		answer := answers[name]
		if len(answer) == 0 {
			continue
		}
		switch answer["type"] {
		case "noul":
			signals[name] = floatOr(answer["noul"], 0)
		case "score":
			signals[name] = floatOr(answer["score"], 0)
		case "choice":
			signals[name] = answer["choice"]
		}
	}
	return signals
}

// hazardProbabilities is the per-category probability, taking the stronger of the choice question
// and its sentinel.
func hazardProbabilities(p *Policy, surface Surface, answers Answers) (map[string]float64, map[string]float64, map[string]bool) {
	probabilities := map[string]float64{}
	confidences := map[string]float64{}
	sentinelSourced := map[string]bool{}

	hazard := answers[Hazard]
	hazardConfidence := floatOr(hazard["confidence"], 1.0)
	for label, value := range mapOf(hazard["probabilities"]) {
		if label == NoneLabel {
			continue
		}
		probabilities[label] = floatOr(value, 0)
		confidences[label] = hazardConfidence
	}

	for _, cat := range p.Sentinels(surface) {
		answer := answers[SentinelPrefix+cat.ID]
		if len(answer) == 0 {
			continue
		}
		value := floatOr(answer["noul"], 0)
		if value >= probabilities[cat.ID] {
			probabilities[cat.ID] = value
			sentinelSourced[cat.ID] = true
			// A noul reports belief, not uncertainty: 0.4 means "40% likely", which the threshold
			// already accounts for. It carries no confidence of its own, so the request-level
			// confidence stands in. Reading distance from 0.5 as doubt would double-count the
			// probability and send every mid-range sentinel to review whatever the threshold says.
			confidences[cat.ID] = hazardConfidence
		}
	}
	return probabilities, confidences, sentinelSourced
}

func finding(cat *Category, surface Surface, probability, confidence float64, sentinel, uncorroborated bool) (Finding, bool) {
	bands, ok := cat.Threshold(surface)
	if !ok {
		return Finding{}, false
	}
	var action Action
	switch {
	case probability >= bands.Block:
		action = Block
	case probability >= bands.Review:
		action = Review
	case probability >= bands.Flag:
		action = Flag
	default:
		return Finding{}, false
	}

	var notes []string
	if categoryRoutes[cat.Route] {
		notes = append(notes, fmt.Sprintf("handled by %s", cat.Route))
	}
	// A sentinel alone, which the hazard choice does not back, answers at its own band: it may
	// still reach block by crossing the block band, but never_below does not lift it there.
	switch {
	case cat.NeverBelow != "" && uncorroborated && probability < bands.Block:
		notes = append(notes, fmt.Sprintf("uncorroborated sentinel: never below %s not applied", cat.NeverBelow))
	case cat.NeverBelow != "":
		action = Stronger(action, cat.NeverBelow)
		notes = append(notes, fmt.Sprintf("never below %s", cat.NeverBelow))
	}
	source := "category"
	if sentinel {
		source = "sentinel"
	}
	return Finding{
		Category:       cat.ID,
		Name:           cat.Name,
		Probability:    probability,
		Confidence:     confidence,
		Action:         action,
		Severity:       float64(cat.BaseSeverity),
		Refs:           cat.Refs,
		Source:         source,
		Notes:          notes,
		Uncorroborated: uncorroborated,
		weak:           uncorroborated && probability < bands.Block,
	}, true
}

func matches(op string, value, target any) bool {
	if op == "==" || op == "!=" {
		equal := pyStr(value) == pyStr(target)
		if op == "==" {
			return equal
		}
		return !equal
	}
	left, okL := toFloat(value)
	right, okR := toFloat(target)
	if !okL || !okR {
		return false
	}
	switch op {
	case ">=":
		return left >= right
	case "<=":
		return left <= right
	case ">":
		return left > right
	case "<":
		return left < right
	}
	return false
}

func applyRules(p *Policy, surface Surface, findings []Finding, signals map[string]any, probabilities map[string]float64) ([]Finding, []Action, []string) {
	var floors []Action
	applied := []string{}

	for _, rule := range p.Rules {
		value, ok := signals[rule.Signal]
		if !ok || !matches(rule.Op, value, rule.Value) {
			continue
		}
		applied = append(applied, rule.ID)

		if added := rule.AddFinding; added != "" && !hasFinding(findings, added) {
			if cat, known := p.Categories[added]; known && cat.Applies(surface) {
				findings = append(findings, Finding{
					Category:    cat.ID,
					Name:        cat.Name,
					Probability: probabilities[cat.ID],
					Confidence:  1.0,
					Action:      Flag,
					Severity:    float64(cat.BaseSeverity),
					Refs:        cat.Refs,
					Source:      "rule:" + rule.ID,
					Notes:       []string{"raised by " + rule.ID},
				})
			}
		}

		steps := rule.Upgrade - rule.Downgrade
		if steps != 0 || rule.CapAction != "" {
			for i, f := range findings {
				if !rule.ExceptCategories[f.Category] {
					findings[i] = adjust(f, steps, rule.CapAction, rule.ID, p)
				}
			}
		}
		if rule.FloorAction != "" {
			floors = append(floors, rule.FloorAction)
		}
	}
	return findings, floors, applied
}

func hasFinding(findings []Finding, category string) bool {
	for _, f := range findings {
		if f.Category == category {
			return true
		}
	}
	return false
}

func adjust(f Finding, steps int, cap Action, ruleID string, p *Policy) Finding {
	action := f.Action
	if steps != 0 {
		action = Shift(action, steps)
	}
	if steps < 0 {
		// A softening rule lowers the response, it does not erase the record: a finding that
		// fired stays visible at "flag" so the deployment can still count and audit it.
		action = Stronger(action, Flag)
	}
	if cap != "" {
		action = Weaker(action, cap)
	}
	if nb := p.Categories[f.Category].NeverBelow; nb != "" && !f.weak {
		action = Stronger(action, nb)
	}
	if action == f.Action {
		return f
	}
	notes := append(append([]string(nil), f.Notes...), fmt.Sprintf("%s: %s -> %s", ruleID, f.Action, action))
	f.Action = action
	f.Notes = notes
	return f
}

func overallConfidence(answers Answers, findings []Finding, confidences map[string]float64) float64 {
	if len(findings) > 0 {
		lowest := 1.0
		for i, f := range findings {
			c, ok := confidences[f.Category]
			if !ok {
				c = 1.0
			}
			if i == 0 || c < lowest {
				lowest = c
			}
		}
		return lowest
	}
	return floatOr(answers[Hazard]["confidence"], 1.0)
}

// confidenceGate escalates toward review, because a low-confidence answer is not evidence of safety.
func confidenceGate(p *Policy, action Action, confidence float64, findings []Finding, probabilities map[string]float64, surface Surface,
	choice map[string]any, signals map[string]any, minChoice float64) (Action, bool) {
	if confidence >= p.MinConfidence() || p.OnLowConfidence() != "escalate" {
		return action, false
	}
	opts := p.ConfidenceGate()
	if intent, ok := signals["intent"].(string); ok && opts.SkipWhenIntent[intent] && confidence >= opts.SkipMinConfidence {
		return action, false
	}
	if opts.NeedsCorroboration {
		// Only what the hazard choice backs counts: a sentinel on its own is already answered at
		// its own band, and is not a reason to hold on top of that.
		var backed []Finding
		for _, f := range findings {
			if !f.Uncorroborated {
				backed = append(backed, f)
			}
		}
		findings = backed
		corroborated := map[string]float64{}
		for cid, pr := range probabilities {
			if floatOr(choice[cid], 0) >= minChoice {
				corroborated[cid] = pr
			}
		}
		probabilities = corroborated
	}
	nearMiss := false
	for cid, probability := range probabilities {
		cat, ok := p.Categories[cid]
		if !ok {
			continue
		}
		flag := 1.0
		if bands, has := cat.Threshold(surface); has {
			flag = bands.Flag
		}
		if probability >= flag*0.5 {
			nearMiss = true
			break
		}
	}
	if len(findings) == 0 && !nearMiss {
		return action, false
	}
	if Rank(action) >= Rank(Review) {
		return action, false
	}
	return Review, true
}

// route says how the deployment should handle the content, given the decision and the hazard.
// The action says whether the content goes out; the route says what to do about it.
func route(p *Policy, findings []Finding, action Action) Route {
	var hazardRoute Route
	for _, f := range findings {
		// Only a finding that still counts sets the handling. One a rule capped to allow is a
		// record, and a sentinel the hazard choice does not back, still at flag, must not replace an
		// ordinary answer with a crisis message.
		if Rank(f.Action) < Rank(Flag) || (f.Uncorroborated && f.Action == Flag) {
			continue
		}
		if cat, ok := p.Categories[f.Category]; ok && categoryRoutes[cat.Route] {
			hazardRoute = cat.Route
			break
		}
	}
	if hazardRoute == RouteCrisisSupport && action != Allow {
		return RouteCrisisSupport
	}
	if action == Block {
		return RouteSafeResponse
	}
	if action == Allow {
		return RouteDeliver
	}
	if hazardRoute == RouteRedact || hazardRoute == RouteGuide {
		return hazardRoute
	}
	if action == Review {
		return RouteHumanReview
	}
	return RouteDeliver
}

func severity(signals map[string]any, findings []Finding) float64 {
	if reported, ok := signals["severity"].(float64); ok {
		return reported
	}
	highest := 0.0
	for _, f := range findings {
		highest = max(highest, f.Severity)
	}
	return highest
}
