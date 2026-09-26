package guardrail

import (
	"context"
	"fmt"
	"slices"
)

// History is for understanding the current turn, never for convicting it.
//
// A classifier that reads a violation in the history tends to hand the same label to whatever comes
// next: an apology, a question about the law, a request for the weather. Two things keep that out:
//
//   - The input check reads only the current message, so the past can never block a question.
//   - A reply in a risky session is also read against the earlier turns, and it is held for them
//     only when it itself completes an earlier harmful request: the next step, more detail, a
//     translation or a fictional retelling of it. A reply that merely follows on from the earlier
//     turns, or refers back to them to apologise, ask about the law or change the subject, is not.
//
// The judgment lands on what is about to be delivered, not on a guess about intent: if a follow-up
// is harmless, so is the answer to it, and there is nothing to hold.

// MultiturnMode says how earlier turns may affect a later verdict.
type MultiturnMode int

const (
	// MultiturnAttribute holds a turn only for what that turn or its reply does. Earlier risk
	// decides when a reply is also read in context, and never holds a clean turn by itself.
	MultiturnAttribute MultiturnMode = iota
	// MultiturnFloor is the earlier behaviour: after a conversation-level review or a block, every
	// verdict for the next turns is raised to a floor, clean ones included.
	MultiturnFloor
)

// ContextCheck tunes the in-context output check.
type ContextCheck struct {
	// Always reads every reply in context whenever there is history, not only in risky sessions.
	Always bool
	// Never turns the in-context check off.
	Never bool
	// WatchRisk is the session risk from which replies are also read in context; zero means 0.2.
	// Risk decays on every check, not every turn, so on its own it covers little more than the next
	// check; the turn-counted watch below is what carries it forward.
	WatchRisk float64
	// Attribution is how sure Jev must be that the reply completes an earlier harmful request
	// before the in-context findings count; zero means 0.5.
	Attribution float64
}

// WithheldPlaceholder stands in the transcript for a message the guardrail withheld. The attempt
// stays visible to the conversation check, so repeated tries still show as a pattern, but its text
// does not: neither Jev nor the model reads a blocked request again.
const WithheldPlaceholder = "[message withheld by the safety check]"

// Names of the questions the in-context check adds.
const (
	ContextCompletes   = "ctx_completes"
	ContextDisengages  = "ctx_disengages"
	contextEvaluating  = "assistant_reply_in_context"
	defaultWatchRisk   = 0.2
	defaultAttribution = 0.5
)

const contextHazardInstructions = "Read the assistant reply together with the earlier turns. Which hazard category best describes harmful content that the reply itself supplies, once its meaning is read in the light of the earlier turns? Judge only what the reply provides: harm that appears only in earlier turns or in the user's messages does not count. Choose 'none' if the reply supplies nothing harmful."

const completesInstructions = "Read with the earlier turns, the assistant reply supplies harmful content or completes a harmful request from an earlier turn: for example the next step of a procedure, more detail on it, or a rephrasing, translation, summary or fictional retelling of it."

const disengagesInstructions = "The latest user message refers to earlier harmful content only to step away from it: to withdraw it or apologise, to ask about the law, consequences, prevention, safety or reporting, to ask why a request was refused, or to change the subject."

// ContextQuestions is the question set for reading a reply against the earlier turns.
func ContextQuestions(p *Policy) (Questions, error) {
	categories := p.ForSurface(SurfaceOutput)
	if len(categories) == 0 {
		return nil, fmt.Errorf("policy %q has no categories for surface %q", p.ID, SurfaceOutput)
	}
	instructions := contextHazardInstructions
	if p.ContentNote != "" {
		instructions += " " + p.ContentNote
	}
	labels := [][2]string{{NoneLabel, noneDescription}}
	for _, c := range categories {
		labels = append(labels, [2]string{c.ID, c.Description})
	}
	return Questions{
		Hazard:            {Type: "choice", Instructions: instructions, Criteria: orderedObject(labels)},
		ContextCompletes:  {Type: "noul", Instructions: completesInstructions},
		ContextDisengages: {Type: "noul", Instructions: disengagesInstructions},
	}, nil
}

// OutputInContextState is the state for reading a reply against the earlier turns.
func OutputInContextState(reply, userMessage string, earlier []Turn, metadata map[string]any) State {
	state := State{
		"evaluating":      contextEvaluating,
		"earlier_turns":   append([]Turn{}, earlier...),
		"assistant_reply": reply,
	}
	if userMessage != "" {
		state["user_message"] = userMessage
	}
	if len(metadata) > 0 {
		state["deployment_context"] = copyMap(metadata)
	}
	return state
}

// ContextRead is what the in-context check found, recorded on the verdict for audit.
type ContextRead struct {
	// Ran is false when the check did not run for this verdict.
	Ran bool `json:"ran"`
	// Attributed is true when the reply was found to complete an earlier harmful request, so the
	// in-context findings count against it.
	Attributed bool    `json:"attributed"`
	Completes  float64 `json:"completes"`
	Disengages float64 `json:"disengages"`
	// Categories fired in context, whether or not they were attributed.
	Categories []string `json:"categories"`
	Error      string   `json:"error,omitempty"`
}

type contextResult struct {
	verdict    Verdict
	completes  float64
	disengages float64
	err        error
}

func (g *Guard) contextCheckApplies(session *Session, earlier []Turn) bool {
	c := g.opts.ContextCheck
	if c.Never || g.opts.Multiturn == MultiturnFloor || len(earlier) == 0 {
		return false
	}
	if c.Always {
		return true
	}
	threshold := c.WatchRisk
	if threshold <= 0 {
		threshold = defaultWatchRisk
	}
	return session != nil && session.Watching(threshold)
}

func (g *Guard) checkInContext(ctx context.Context, reply, userMessage string, earlier []Turn, metadata map[string]any, model string) contextResult {
	questions, err := ContextQuestions(g.Policy)
	if err != nil {
		return contextResult{err: err}
	}
	answer, err := g.call(ctx, OutputInContextState(reply, userMessage, earlier, metadata), questions, model)
	if err != nil {
		return contextResult{err: err}
	}
	return contextResult{
		verdict:    Decide(g.Policy, SurfaceOutput, answer.Answers, DecideOptions{Model: answer.Model, Usage: answer.Usage}),
		completes:  floatOr(answer.Answers[ContextCompletes]["noul"], 0),
		disengages: floatOr(answer.Answers[ContextDisengages]["noul"], 0),
	}
}

// attribute merges the in-context findings into the standalone verdict, but only when the reply
// itself completes an earlier harmful request. Otherwise they are recorded and change nothing.
func (g *Guard) attribute(v Verdict, res contextResult) Verdict {
	read := ContextRead{Ran: true, Completes: res.completes, Disengages: res.disengages}
	if res.err != nil {
		// The standalone check still stands; an outage of the second read is not evidence.
		read.Error = res.err.Error()
		v.Context = &read
		v.AppliedRules = append(append([]string(nil), v.AppliedRules...), "context-unavailable")
		return v
	}
	var fired []Finding
	for _, f := range res.verdict.Findings {
		if Rank(f.Action) >= Rank(Flag) {
			fired = append(fired, f)
			read.Categories = append(read.Categories, f.Category)
		}
	}
	threshold := g.opts.ContextCheck.Attribution
	if threshold <= 0 {
		threshold = defaultAttribution
	}
	read.Attributed = len(fired) > 0 && res.completes >= threshold && res.completes >= res.disengages
	v.Context = &read
	if len(fired) == 0 {
		return v
	}
	if !read.Attributed {
		v.AppliedRules = append(append([]string(nil), v.AppliedRules...), "context-not-attributed")
		return v
	}

	findings := append([]Finding(nil), v.Findings...)
	for _, f := range fired {
		f.Source = "context:" + f.Source
		f.Notes = append(append([]string(nil), f.Notes...), "completes an earlier request")
		if i := slices.IndexFunc(findings, func(x Finding) bool { return x.Category == f.Category }); i >= 0 {
			if Rank(f.Action) > Rank(findings[i].Action) {
				findings[i] = f
			}
			continue
		}
		findings = append(findings, f)
	}
	sortFindings(findings)
	action := v.Action
	for _, f := range findings {
		action = Stronger(action, f.Action)
	}
	v.Findings = findings
	v.Action = action
	v.Route = route(g.Policy, findings, action)
	v.Severity = max(v.Severity, res.verdict.Severity)
	v.AppliedRules = append(append([]string(nil), v.AppliedRules...), "context-attributed")
	return v
}

// -- session support -------------------------------------------------------------------

// Record appends a turn given the verdict on it. A turn the guardrail withheld is kept as
// WithheldPlaceholder, so the attempt is remembered but its text is never read again.
func (s *Session) Record(role, content string, v Verdict) {
	if !v.Deliverable() {
		content = WithheldPlaceholder
	}
	s.AddTurn(role, content)
}

// ModelHistory is the transcript to send to the chat model: withheld turns are left out, so the
// model never sees a blocked request, not even as a placeholder it might try to answer.
func (s *Session) ModelHistory() []Turn {
	var out []Turn
	for _, t := range s.Turns {
		if t.Content != WithheldPlaceholder {
			out = append(out, t)
		}
	}
	return out
}

// Watching reports whether replies should be read in context. Any of three keeps the watch on:
//
//   - a conversation verdict of review or worse, or a block, in the last CarryTurns completed
//     turns (counted by Advance, the same counter MultiturnFloor uses for its floor);
//   - a risk that has not yet decayed below threshold;
//   - a withheld turn still inside the transcript window.
//
// Watching never holds anything by itself. It only decides whether a reply is also read against
// the earlier turns, which is what costs a second request.
func (s *Session) Watching(threshold float64) bool {
	if s.floorLeft > 0 || s.Risk >= threshold {
		return true
	}
	return slices.ContainsFunc(s.Turns, func(t Turn) bool { return t.Content == WithheldPlaceholder })
}

func sortFindings(findings []Finding) {
	slices.SortStableFunc(findings, func(a, b Finding) int {
		if Rank(a.Action) != Rank(b.Action) {
			return Rank(b.Action) - Rank(a.Action)
		}
		switch {
		case a.Probability > b.Probability:
			return -1
		case a.Probability < b.Probability:
			return 1
		}
		return 0
	})
}
