package guardrail

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Options configures a Guard. Every field is optional.
type Options struct {
	// Policy defaults to the bundled standard-v1 pack.
	Policy *Policy
	// Transport is how to reach Jev. It defaults to an HTTPTransport built from HTTP on first use,
	// so constructing a Guard never needs an API key. Pass a RecordedTransport to work offline.
	Transport Transport
	// HTTP configures the default transport; ignored when Transport is set.
	HTTP HTTPOptions
	// Cache is an optional verdict cache. Identical content skips the round trip.
	Cache VerdictCache
	// CacheSurfaces are the surfaces the cache applies to; nil means input and output.
	// Conversation state changes every turn, so caching it buys nothing and only costs memory.
	CacheSurfaces []Surface
	// Prefilter is an optional deterministic check run before Jev; a verdict from it settles the
	// check without a network call.
	Prefilter Prefilter
	// Observer is called with every verdict, including cached and degraded ones. This is the
	// metrics hook; keep it fast.
	Observer func(Verdict)
	// RaiseOnError returns the error instead of a degraded verdict when Jev is unreachable.
	RaiseOnError bool
	// Timeout is the per-call timeout; zero leaves it to the transport.
	Timeout time.Duration
	// Multiturn says how earlier turns may affect a later verdict; see MultiturnMode.
	Multiturn MultiturnMode
	// ContextCheck tunes the in-context output check that MultiturnAttribute uses.
	ContextCheck ContextCheck
	// ReviewHandling says what a review verdict does to the content; see ReviewAsAudit.
	ReviewHandling ReviewHandling
}

// ReviewHandling says what a review verdict does to the content.
type ReviewHandling int

const (
	// ReviewHold withholds content at review until a person has looked at it.
	ReviewHold ReviewHandling = iota
	// ReviewAsAudit is for realtime chat, where nobody can look before the reply is due: review
	// delivers the content and queues it for a person afterwards. Only block stops content, and
	// crisis support still replaces it. A degraded verdict is not affected: when Jev could not be
	// reached nothing was checked, so a fail-closed surface still holds.
	ReviewAsAudit
)

// CheckOptions are per-check details. Fields that do not apply to a check are ignored.
type CheckOptions struct {
	// Metadata is deployment context put in front of Jev. It wins over the session's.
	Metadata map[string]any
	// Model overrides the Jev model for this check.
	Model string
	// Session carries risk across turns; see Session.
	Session *Session
	// UserMessage is the message an assistant reply answered (output checks).
	UserMessage string
	// Context is the retrieved passages the reply was meant to be based on (output checks). It
	// enables the groundedness signal, which catches claims the context does not support.
	Context []string
	// Quick asks only the sentinel questions (output checks). It is what mid-stream checks use on
	// incomplete text; a complete reply should get the full set.
	Quick bool
	// History is the conversation before this exchange (CheckTurn). It defaults to the session's.
	History []Turn
}

// Guard checks chatbot content against a policy pack using Jev. It is safe for concurrent use;
// make one per process.
type Guard struct {
	Policy *Policy

	opts          Options
	cacheSurfaces map[Surface]bool

	mu        sync.Mutex
	transport Transport
}

// New builds a Guard.
func New(opts Options) *Guard {
	g := &Guard{Policy: opts.Policy, opts: opts, transport: opts.Transport, cacheSurfaces: map[Surface]bool{}}
	if g.Policy == nil {
		g.Policy = MustBundledPolicy()
	}
	surfaces := opts.CacheSurfaces
	if surfaces == nil {
		surfaces = []Surface{SurfaceInput, SurfaceOutput}
	}
	for _, s := range surfaces {
		g.cacheSurfaces[s] = true
	}
	return g
}

// Transport returns the transport, building the default one on first use.
func (g *Guard) Transport() (Transport, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.transport == nil {
		t, err := NewHTTPTransport(g.opts.HTTP)
		if err != nil {
			return nil, err
		}
		g.transport = t
	}
	return g.transport, nil
}

// CheckInput checks a user message before it reaches the model.
func (g *Guard) CheckInput(ctx context.Context, content string, opts *CheckOptions) (Verdict, error) {
	o := orEmpty(opts)
	state := InputState(content, mergedMetadata(o.Metadata, o.Session))
	return g.run(ctx, SurfaceInput, state, false, o.Model, o.Session, SubsetFull)
}

// CheckOutput checks an assistant reply before it reaches the user.
//
// In a session with recent risk, a complete reply is also read against the earlier turns, and only
// a reply that itself completes an earlier harmful request is held for it; see MultiturnAttribute.
func (g *Guard) CheckOutput(ctx context.Context, reply string, opts *CheckOptions) (Verdict, error) {
	o := orEmpty(opts)
	metadata := mergedMetadata(o.Metadata, o.Session)
	state := OutputState(reply, o.UserMessage, o.Context, metadata)
	subset := SubsetFull
	if o.Quick {
		subset = SubsetSentinels
	}

	earlier := o.History
	if len(earlier) == 0 && o.Session != nil {
		earlier = o.Session.History()
	}
	if o.Quick || !g.contextCheckApplies(o.Session, earlier) {
		return g.run(ctx, SurfaceOutput, state, len(o.Context) > 0, o.Model, o.Session, subset)
	}

	// Both requests go out together, so the in-context read adds no latency, and the standalone
	// check never sees the history: its answer stays uncontaminated by what came before.
	inContext := make(chan contextResult, 1)
	go func() {
		inContext <- g.checkInContext(ctx, reply, o.UserMessage, earlier, metadata, o.Model)
	}()
	v, err := g.evaluate(ctx, SurfaceOutput, state, len(o.Context) > 0, o.Model, subset)
	res := <-inContext
	if err != nil {
		return Verdict{}, err
	}
	return g.finish(g.attribute(v, res), o.Session), nil
}

// CheckConversation checks a whole conversation for patterns no single turn reveals.
//
// Multi-turn jailbreaks look harmless turn by turn: the escalation is the attack. This check
// reads the transcript as one state, and belongs off the critical path.
//
// A transcript with nothing in it but withheld turns is not sent: there is no content to read, and
// Jev, asked to judge only omissions, answers from what they might have been.
func (g *Guard) CheckConversation(ctx context.Context, turns []Turn, opts *CheckOptions) (Verdict, error) {
	o := orEmpty(opts)
	if !slices.ContainsFunc(turns, func(t Turn) bool { return t.Content != WithheldPlaceholder }) {
		return g.finish(Verdict{
			Action: Allow, Surface: SurfaceConversation, Route: RouteDeliver, Confidence: 1,
			AppliedRules: []string{"nothing-to-read"}, PolicyID: g.Policy.QualifiedID(),
		}, o.Session), nil
	}
	state := ConversationState(turns, mergedMetadata(o.Metadata, o.Session))
	return g.run(ctx, SurfaceConversation, state, false, o.Model, o.Session, SubsetFull)
}

// CheckTurn runs every applicable check for one exchange and returns them keyed by surface.
// The conversation check runs only when there is history to read.
func (g *Guard) CheckTurn(ctx context.Context, userMessage, reply string, opts *CheckOptions) (map[Surface]Verdict, error) {
	o := orEmpty(opts)
	verdicts := map[Surface]Verdict{}

	in, err := g.CheckInput(ctx, userMessage, &CheckOptions{Metadata: o.Metadata, Session: o.Session})
	if err != nil {
		return verdicts, err
	}
	verdicts[SurfaceInput] = in

	out, err := g.CheckOutput(ctx, reply, &CheckOptions{
		Metadata: o.Metadata, Session: o.Session, UserMessage: userMessage, Context: o.Context,
	})
	if err != nil {
		return verdicts, err
	}
	verdicts[SurfaceOutput] = out

	history := o.History
	if len(history) == 0 && o.Session != nil {
		history = o.Session.History()
	}
	if len(history) > 0 {
		turns := append(append([]Turn(nil), history...), Turn{Role: "user", Content: userMessage}, Turn{Role: "assistant", Content: reply})
		conv, err := g.CheckConversation(ctx, turns, &CheckOptions{Metadata: o.Metadata, Session: o.Session})
		if err != nil {
			return verdicts, err
		}
		verdicts[SurfaceConversation] = conv
	}
	return verdicts, nil
}

// Preview is the exact request body that would be sent, without sending it.
type Preview struct {
	State     State     `json:"state"`
	Questions Questions `json:"questions"`
}

// Preview returns the request for a check without sending it. It needs no API key.
func (g *Guard) Preview(surface Surface, state State, hasContext bool, subset string) (Preview, error) {
	questions, err := BuildQuestions(g.Policy, surface, hasContext, subset)
	return Preview{State: state, Questions: questions}, err
}

// -- internals ----------------------------------------------------------------

func (g *Guard) run(ctx context.Context, surface Surface, state State, hasContext bool, model string, session *Session, subset string) (Verdict, error) {
	v, err := g.evaluate(ctx, surface, state, hasContext, model, subset)
	if err != nil {
		return Verdict{}, err
	}
	return g.finish(v, session), nil
}

// evaluate reaches a verdict for one state without touching the session or the observer.
func (g *Guard) evaluate(ctx context.Context, surface Surface, state State, hasContext bool, model string, subset string) (Verdict, error) {
	if g.opts.Prefilter != nil {
		if decided, ok := g.opts.Prefilter.Decide(g.Policy, surface, state); ok {
			return decided, nil
		}
	}

	var key string
	if g.opts.Cache != nil && g.cacheSurfaces[surface] {
		key = CacheKey(g.Policy.QualifiedID(), surface, state, subset, nil)
		if hit, ok := g.opts.Cache.Get(key); ok {
			return hit, nil
		}
	}

	questions, err := BuildQuestions(g.Policy, surface, hasContext, subset)
	if err != nil {
		return Verdict{}, err
	}
	started := time.Now()
	reply, err := g.call(ctx, state, questions, model)
	if err != nil {
		if g.opts.RaiseOnError {
			return Verdict{}, err
		}
		return ErrorVerdict(g.Policy, surface, err, elapsedMS(started)), nil
	}

	verdict := Decide(g.Policy, surface, reply.Answers, DecideOptions{
		Model: reply.Model, Usage: reply.Usage, LatencyMS: elapsedMS(started),
	})
	verdict.Partial = subset != SubsetFull
	if key != "" {
		g.opts.Cache.Put(key, verdict)
	}
	return verdict, nil
}

func (g *Guard) call(ctx context.Context, state State, questions Questions, model string) (Reply, error) {
	transport, err := g.Transport()
	if err != nil {
		return Reply{}, err
	}
	return transport.SystemOne(ctx, state, questions, CallOptions{Model: model, Timeout: g.opts.Timeout})
}

// finish applies the session floor (MultiturnFloor only), tells the session, and emits to the
// observer.
func (g *Guard) finish(v Verdict, session *Session) Verdict {
	v = g.audit(v)
	if session != nil {
		if floor := session.Floor(); floor != Allow && g.opts.Multiturn == MultiturnFloor {
			id := session.ID
			if id == "" {
				id = "unnamed"
			}
			v = WithFloor(g.Policy, v, floor, "session-floor:"+id)
		}
		session.Observe(v)
	}
	if g.opts.Observer != nil {
		g.opts.Observer(v)
	}
	return v
}

// audit marks what a person should look at later, and under ReviewAsAudit lets a review through.
func (g *Guard) audit(v Verdict) Verdict {
	switch {
	case Rank(v.Action) >= Rank(Review):
		v.Audit = "priority"
	case v.Action == Flag:
		v.Audit = "sample"
	}
	if g.opts.ReviewHandling == ReviewAsAudit && v.Route == RouteHumanReview && !v.Degraded {
		v.Route = RouteDeliverAndAudit
	}
	return v
}

// mergedMetadata merges caller metadata over the session's, so an explicit value always wins.
func mergedMetadata(metadata map[string]any, session *Session) map[string]any {
	if session == nil {
		return metadata
	}
	merged := session.Metadata()
	for k, v := range metadata {
		merged[k] = v
	}
	return merged
}

func orEmpty(o *CheckOptions) CheckOptions {
	if o == nil {
		return CheckOptions{}
	}
	return *o
}

func elapsedMS(since time.Time) float64 {
	return float64(time.Since(since)) / float64(time.Millisecond)
}
