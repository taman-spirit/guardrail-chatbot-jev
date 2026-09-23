package guardrail

import (
	"context"
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
}

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
func (g *Guard) CheckOutput(ctx context.Context, reply string, opts *CheckOptions) (Verdict, error) {
	o := orEmpty(opts)
	state := OutputState(reply, o.UserMessage, o.Context, mergedMetadata(o.Metadata, o.Session))
	subset := SubsetFull
	if o.Quick {
		subset = SubsetSentinels
	}
	return g.run(ctx, SurfaceOutput, state, len(o.Context) > 0, o.Model, o.Session, subset)
}

// CheckConversation checks a whole conversation for patterns no single turn reveals.
//
// Multi-turn jailbreaks look harmless turn by turn: the escalation is the attack. This check
// reads the transcript as one state, and belongs off the critical path.
func (g *Guard) CheckConversation(ctx context.Context, turns []Turn, opts *CheckOptions) (Verdict, error) {
	o := orEmpty(opts)
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
	if history == nil && o.Session != nil {
		history = o.Session.History()
	}
	if len(history) > 0 {
		turns := append(append([]Turn(nil), history...), Turn{"user", userMessage}, Turn{"assistant", reply})
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
	if g.opts.Prefilter != nil {
		if decided, ok := g.opts.Prefilter.Decide(g.Policy, surface, state); ok {
			return g.finish(decided, session), nil
		}
	}

	var key string
	if g.opts.Cache != nil && g.cacheSurfaces[surface] {
		key = CacheKey(g.Policy.QualifiedID(), surface, state, subset, nil)
		if hit, ok := g.opts.Cache.Get(key); ok {
			return g.finish(hit, session), nil
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
		return g.finish(ErrorVerdict(g.Policy, surface, err, elapsedMS(started)), session), nil
	}

	verdict := Decide(g.Policy, surface, reply.Answers, DecideOptions{
		Model: reply.Model, Usage: reply.Usage, LatencyMS: elapsedMS(started),
	})
	verdict.Partial = subset != SubsetFull
	if key != "" {
		g.opts.Cache.Put(key, verdict)
	}
	return g.finish(verdict, session), nil
}

func (g *Guard) call(ctx context.Context, state State, questions Questions, model string) (Reply, error) {
	transport, err := g.Transport()
	if err != nil {
		return Reply{}, err
	}
	return transport.SystemOne(ctx, state, questions, CallOptions{Model: model, Timeout: g.opts.Timeout})
}

// finish applies the session floor, tells the session, and emits to the observer.
func (g *Guard) finish(v Verdict, session *Session) Verdict {
	if session != nil {
		if floor := session.Floor(); floor != Allow {
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
