//go:build replay

package guardrail

// Offline replay of recorded Jev answers under candidate policy variants. No network:
//
//	REPLAY_DIRS='/path/live-mt*' go test -tags replay -run TestReplayVariants -v ./
//
// Every standalone input and output answer recorded by the live runs is decided again under each
// variant. Harmless texts come from examples/multiturn-live.jsonl; violations are the labelled cases
// in cases-input.jsonl and cases-output.jsonl expected at review or block.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

type sample struct {
	surface Surface
	text    string
	answers Answers
	harmful bool
}

func loadSamples(t *testing.T) []sample {
	pattern := os.Getenv("REPLAY_DIRS")
	if pattern == "" {
		t.Skip("REPLAY_DIRS is not set")
	}
	harmful := map[string]bool{}
	for _, set := range []string{"../examples/cases-input.jsonl", "../examples/cases-output.jsonl"} {
		for _, c := range readRows(t, set) {
			a := Action(c["expected_action"].(string))
			harmful[c["text"].(string)] = Rank(a) >= Rank(Review)
		}
	}
	benign := map[string]bool{}
	for _, c := range readRows(t, "../examples/multiturn-live.jsonl") {
		if w, ok := c["expected_withheld"].(bool); ok && !w {
			benign[c["user_message"].(string)] = true
			if r, ok := c["reply"].(string); ok {
				benign[r] = true
			}
		}
	}
	dirs, _ := filepath.Glob(strings.TrimRight(pattern, "/"))
	var out []sample
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, "answers.jsonl")); err != nil {
			continue
		}
		for _, row := range readRows(t, filepath.Join(d, "answers.jsonl")) {
			state := row["state"].(map[string]any)
			var s sample
			switch state["evaluating"] {
			case "user_message":
				s.surface, s.text = SurfaceInput, state["user_message"].(string)
			case "assistant_reply":
				s.surface, s.text = SurfaceOutput, state["assistant_reply"].(string)
			default:
				continue
			}
			isHarm, labelled := harmful[s.text]
			switch {
			case labelled && isHarm:
				s.harmful = true
			case benign[s.text]:
			default:
				continue
			}
			raw, _ := json.Marshal(row["answers"])
			_ = json.Unmarshal(raw, &s.answers)
			out = append(out, s)
		}
	}
	return out
}

func readRows(t *testing.T, path string) []map[string]any {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var r map[string]any
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			rows = append(rows, r)
		}
	}
	return rows
}

type variant struct {
	name  string
	edit  func(d map[string]any)
	reask bool
}

func policyWith(t *testing.T, edit func(map[string]any)) *Policy {
	var d map[string]any
	data, _ := bundledPacks.ReadFile("policies/standard-v1.json")
	_ = json.Unmarshal(data, &d)
	if edit != nil {
		edit(d)
	}
	p, err := NewPolicy(d)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func defaults(d map[string]any) map[string]any { return d["defaults"].(map[string]any) }

func corr(d map[string]any) map[string]any {
	return defaults(d)["sentinel_corroboration"].(map[string]any)
}

func gate(d map[string]any) map[string]any {
	g, ok := defaults(d)["confidence_gate"].(map[string]any)
	if !ok {
		g = map[string]any{}
		defaults(d)["confidence_gate"] = g
	}
	return g
}

func chain(fs ...func(map[string]any)) func(map[string]any) {
	return func(d map[string]any) {
		for _, f := range fs {
			f(d)
		}
	}
}

var (
	weakFlag   = func(d map[string]any) { corr(d)["weak_at_most_flag"] = true }
	gateCorr   = func(d map[string]any) { gate(d)["needs_corroboration"] = true }
	gateIntent = func(d map[string]any) { gate(d)["skip_when_intent"] = []any{"benign"} }
	minConf    = func(v float64) func(map[string]any) {
		return func(d map[string]any) { defaults(d)["min_confidence"] = v }
	}
)

// borderline is a hold that rests on weak evidence: escalated by the confidence gate, or every
// finding that holds it is an uncorroborated sentinel, or the strongest one sits within 1.5x of
// the band it crossed.
func borderline(p *Policy, s sample, v Verdict) bool {
	if v.Deliverable() {
		return false
	}
	if v.HasRule("confidence-gate") {
		return true
	}
	for _, f := range v.Findings {
		if Rank(f.Action) < Rank(Review) {
			continue
		}
		if f.Uncorroborated {
			continue
		}
		bands, _ := p.Categories[f.Category].Threshold(s.surface)
		edge := bands.Review
		if f.Action == Block {
			edge = bands.Block
		}
		if f.Probability >= edge*1.5 {
			return false
		}
	}
	return true
}

// average merges two answer sets for the same text: every probability, noul, score and
// confidence is the mean of the two.
func average(a, b Answers) Answers {
	out := Answers{}
	for name, x := range a {
		y := b[name]
		m := Answer{}
		for k, v := range x {
			m[k] = v
		}
		for _, k := range []string{"noul", "score", "confidence"} {
			if fx, ok := toFloat(x[k]); ok {
				if fy, ok2 := toFloat(y[k]); ok2 {
					m[k] = (fx + fy) / 2
				}
			}
		}
		if px, ok := x["probabilities"].(map[string]any); ok {
			py, _ := y["probabilities"].(map[string]any)
			pm := map[string]any{}
			for label := range px {
				pm[label] = (floatOr(px[label], 0) + floatOr(py[label], 0)) / 2
			}
			for label := range py {
				if _, seen := pm[label]; !seen {
					pm[label] = floatOr(py[label], 0) / 2
				}
			}
			m["probabilities"] = pm
		}
		out[name] = m
	}
	return out
}

func TestReplayVariants(t *testing.T) {
	samples := loadSamples(t)
	byText := map[string][]int{}
	for i, s := range samples {
		k := string(s.surface) + "|" + s.text
		byText[k] = append(byText[k], i)
	}
	variants := []variant{
		{name: "current (after last round)"},
		{name: "A weak sentinel at most flag", edit: weakFlag},
		{name: "B gate needs corroboration", edit: gateCorr},
		{name: "C gate skipped for benign intent", edit: gateIntent},
		{name: "D min_confidence 0.6", edit: minConf(0.6)},
		{name: "D min_confidence 0.55", edit: minConf(0.55)},
		{name: "A+B", edit: chain(weakFlag, gateCorr)},
		{name: "A+C", edit: chain(weakFlag, gateIntent)},
		{name: "A+B+C", edit: chain(weakFlag, gateCorr, gateIntent)},
		{name: "E re-ask borderline holds", reask: true},
		{name: "A+B+C + E", edit: chain(weakFlag, gateCorr, gateIntent), reask: true},
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-34s %-26s %-26s %s\n", "variant", "harmless held (FPR)", "violations caught", "re-asks")
	for _, v := range variants {
		p := policyWith(t, v.edit)
		rng := rand.New(rand.NewSource(11))
		var benign, benignHeld, harm, harmCaught, reasks int
		heldTexts := map[string]int{}
		for i, s := range samples {
			verdict := Decide(p, s.surface, s.answers, DecideOptions{})
			if v.reask && borderline(p, s, verdict) {
				// A fresh answer for the same text, from another recorded sample.
				peers := byText[string(s.surface)+"|"+s.text]
				if len(peers) > 1 {
					j := peers[rng.Intn(len(peers))]
					for j == i {
						j = peers[rng.Intn(len(peers))]
					}
					verdict = Decide(p, s.surface, average(s.answers, samples[j].answers), DecideOptions{})
					reasks++
				}
			}
			if s.harmful {
				harm++
				if Rank(verdict.Action) >= Rank(Review) {
					harmCaught++
				}
			} else {
				benign++
				if !verdict.Deliverable() {
					benignHeld++
					heldTexts[s.text]++
				}
			}
		}
		fmt.Fprintf(&b, "%-34s %5.2f%% (%4d/%4d)       %6.2f%% (%4d/%4d)      %d\n", v.name,
			100*ratio(benignHeld, benign), benignHeld, benign, 100*ratio(harmCaught, harm), harmCaught, harm, reasks)
		if v.name == "current (after last round)" || v.name == "A+B+C + E" {
			type kv struct {
				t string
				n int
			}
			var top []kv
			for k, n := range heldTexts {
				top = append(top, kv{k, n})
			}
			sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
			for _, x := range top[:min(6, len(top))] {
				fmt.Fprintf(&b, "      held %3dx  %.70q\n", x.n, x.t)
			}
		}
	}
	t.Log(b.String())
}

// TestReplayConversation measures the conversation check: how often a harmless conversation is sent
// to review (the operations queue), and how often an escalating one is caught.
func TestReplayConversation(t *testing.T) {
	pattern := os.Getenv("REPLAY_DIRS")
	if pattern == "" {
		t.Skip("REPLAY_DIRS is not set")
	}
	benignLast := map[string]bool{}
	for _, c := range readRows(t, "../examples/multiturn-live.jsonl") {
		if w, ok := c["expected_withheld"].(bool); ok && !w {
			benignLast[c["user_message"].(string)] = true
		}
	}
	escalationLast := map[string]bool{}
	for _, c := range readRows(t, "../examples/conversations.jsonl") {
		turns := c["turns"].([]any)
		last := turns[len(turns)-1].(map[string]any)["content"].(string)
		escalationLast[last] = c["expected_action"].(string) != "allow"
	}
	// A conversation that still carries a violation verbatim is the old floor mode's transcript,
	// where review is right; the new mode sends only placeholders for withheld turns.
	verbatim := map[string]bool{}
	for _, set := range []string{"../examples/cases-input.jsonl", "../examples/cases-output.jsonl"} {
		for _, c := range readRows(t, set) {
			if Rank(Action(c["expected_action"].(string))) >= Rank(Review) {
				verbatim[c["text"].(string)] = true
				if u, ok := c["user_message"].(string); ok {
					verbatim[u] = true
				}
			}
		}
	}
	type conv struct {
		answers Answers
		harmful bool
	}
	var convs []conv
	dirs, _ := filepath.Glob(strings.TrimRight(pattern, "/"))
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, "answers.jsonl")); err != nil {
			continue
		}
		for _, row := range readRows(t, filepath.Join(d, "answers.jsonl")) {
			state := row["state"].(map[string]any)
			if state["evaluating"] != "conversation" {
				continue
			}
			turns := state["turns"].([]any)
			last := turns[len(turns)-1].(map[string]any)["content"].(string)
			harm, isEsc := escalationLast[last]
			if !(benignLast[last] || (isEsc && harm)) {
				continue
			}
			if !isEsc && slices.ContainsFunc(turns, func(x any) bool { return verbatim[x.(map[string]any)["content"].(string)] }) {
				continue
			}
			var a Answers
			raw, _ := json.Marshal(row["answers"])
			_ = json.Unmarshal(raw, &a)
			convs = append(convs, conv{a, isEsc && harm})
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-34s %-30s %s\n", "variant", "harmless sent to review", "escalations at review or worse")
	noEscalation := func(d map[string]any) {
		d["rules"] = append(d["rules"].([]any), map[string]any{
			"id":                "no-escalation-caps-conversation",
			"when":              map[string]any{"signal": "escalation", "op": "<=", "value": 1.0},
			"except_categories": []any{"cse", "ssh"},
			"then":              map[string]any{"cap_action": "flag"},
		})
	}
	escalationVariant := func(value float64, except ...any) func(map[string]any) {
		return func(d map[string]any) {
			d["rules"] = append(d["rules"].([]any), map[string]any{
				"id":                "no-escalation-caps-conversation",
				"when":              map[string]any{"signal": "escalation", "op": "<=", "value": value},
				"except_categories": except,
				"then":              map[string]any{"cap_action": "flag"},
			})
		}
	}
	noSpcInConversation := func(d map[string]any) {
		d["categories"].(map[string]any)["spc"].(map[string]any)["surfaces"] = []any{"output"}
	}
	for _, v := range []variant{
		{name: "current"},
		{name: "A+C", edit: chain(weakFlag, gateIntent)},
		{name: "A+C+F no escalation caps", edit: chain(weakFlag, gateIntent, noEscalation)},
		{name: "A+C+G spc output only", edit: chain(weakFlag, gateIntent, noSpcInConversation)},
		{name: "A+C+F+G", edit: chain(weakFlag, gateIntent, noEscalation, noSpcInConversation)},
		{name: "A+C+G + F(<=0.5)", edit: chain(weakFlag, gateIntent, noSpcInConversation, escalationVariant(0.5, "cse", "ssh"))},
		{name: "A+C+G + F(<=1, also pij)", edit: chain(weakFlag, gateIntent, noSpcInConversation, escalationVariant(1.0, "cse", "ssh", "pij"))},
		{name: "A+C+G + F(<=0.5, also pij)", edit: chain(weakFlag, gateIntent, noSpcInConversation, escalationVariant(0.5, "cse", "ssh", "pij"))},
	} {
		p := policyWith(t, v.edit)
		var n, fp, h, caught int
		for _, c := range convs {
			verdict := Decide(p, SurfaceConversation, c.answers, DecideOptions{})
			if c.harmful {
				h++
				if Rank(verdict.Action) >= Rank(Review) {
					caught++
				}
			} else {
				n++
				if Rank(verdict.Action) >= Rank(Review) {
					fp++
				}
			}
		}
		fmt.Fprintf(&b, "%-34s %6.2f%% (%4d/%4d)          %6.2f%% (%d/%d)\n", v.name, 100*ratio(fp, n), fp, n, 100*ratio(caught, h), caught, h)
		for _, c := range convs {
			verdict := Decide(p, SurfaceConversation, c.answers, DecideOptions{})
			if c.harmful && Rank(verdict.Action) < Rank(Review) {
				fmt.Fprintf(&b, "      missed: escalation=%v findings=%v rules=%v\n", verdict.Signals["escalation"], verdict.Categories(), verdict.AppliedRules)
			}
		}
	}
	t.Log(b.String())
}
