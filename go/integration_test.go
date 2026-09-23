package guardrail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

var ctx = context.Background()

// failing is a transport that is always down.
var failing = TransportFunc(func(context.Context, State, Questions, CallOptions) (Reply, error) {
	return Reply{}, &Error{Msg: "service unavailable"}
})

// -- per-surface fail modes ---------------------------------------------------

func TestInputFailsOpenAndOutputFailsClosed(t *testing.T) {
	// Blocking every user when Jev is down is a self-inflicted outage; the output check has
	// nothing behind it, so it holds.
	p := bundled(t)
	onInput := ErrorVerdict(p, SurfaceInput, &Error{Msg: "down"}, 0)
	if onInput.Action != Allow || !onInput.Degraded {
		t.Fatalf("failing open must still be visible as a degraded verdict: %+v", onInput)
	}
	onOutput := ErrorVerdict(p, SurfaceOutput, &Error{Msg: "down"}, 0)
	if onOutput.Action != Review || onOutput.Deliverable() {
		t.Fatalf("got %+v", onOutput)
	}
}

func TestAStringOnErrorStillAppliesEverywhere(t *testing.T) {
	var pack map[string]any
	data, _ := bundledPacks.ReadFile("policies/standard-v1.json")
	_ = json.Unmarshal(data, &pack)
	pack["defaults"].(map[string]any)["on_error"] = "fail_closed"
	strict, err := NewPolicy(pack)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range Surfaces {
		if got := ErrorVerdict(strict, s, &Error{Msg: "down"}, 0).Action; got != Review {
			t.Fatalf("%s: %s", s, got)
		}
	}
}

func TestRaiseOnErrorReturnsTheError(t *testing.T) {
	_, err := New(Options{Transport: failing, RaiseOnError: true}).CheckInput(ctx, "hi", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestAMissingKeyIsADegradedVerdictNotAPanic(t *testing.T) {
	t.Setenv("JEV_API_KEY", "")
	v, err := New(Options{}).CheckOutput(ctx, "hi", nil)
	if err != nil || !v.Degraded || v.Action != Review || !strings.Contains(v.Error, "No Jev API key") {
		t.Fatalf("%v %+v", err, v)
	}
}

// -- cache ----------------------------------------------------------------------

func TestCacheSparesTheSecondRoundTrip(t *testing.T) {
	transport := NewRecordedTransport(clean)
	guard := New(Options{Transport: transport, Cache: NewLRUCache(0, 0)})
	first, _ := guard.CheckInput(ctx, "cùng một câu hỏi", nil)
	second, _ := guard.CheckInput(ctx, "cùng một câu hỏi", nil)
	if len(transport.Calls()) != 1 || first.Cached || !second.Cached || second.Action != first.Action {
		t.Fatalf("calls=%d first=%+v second=%+v", len(transport.Calls()), first, second)
	}
}

func TestSessionMetadataDoesNotDefeatTheCache(t *testing.T) {
	// The turn number changes every turn; keying on it would mean the cache never hits.
	transport := NewRecordedTransport(clean)
	guard := New(Options{Transport: transport, Cache: NewLRUCache(0, 0)})
	session := NewSession("c0")
	_, _ = guard.CheckInput(ctx, "cùng một câu hỏi", &CheckOptions{Session: session})
	session.AddTurn("user", "cùng một câu hỏi")
	second, _ := guard.CheckInput(ctx, "cùng một câu hỏi", &CheckOptions{Session: session})
	if !second.Cached || len(transport.Calls()) != 1 {
		t.Fatalf("got %+v", second)
	}
}

func TestCacheKeySeparatesContentAndPolicyVersion(t *testing.T) {
	a := CacheKey("p@1", SurfaceInput, State{"user_message": "a"}, "", nil)
	b := CacheKey("p@1", SurfaceInput, State{"user_message": "b"}, "", nil)
	c := CacheKey("p@2", SurfaceInput, State{"user_message": "a"}, "", nil)
	if a == b || a == c {
		t.Fatal("cache keys collide")
	}
}

func TestADegradedVerdictIsNeverCached(t *testing.T) {
	// Otherwise a brief outage becomes a lasting wrong answer for that exact message.
	cache := NewLRUCache(0, 0)
	_, _ = New(Options{Transport: failing, Cache: cache}).CheckInput(ctx, "hello", nil)
	if cache.Len() != 0 {
		t.Fatal("degraded verdict cached")
	}
}

func TestCacheEvictsByCapacity(t *testing.T) {
	cache := NewLRUCache(2, 0)
	guard := New(Options{Transport: NewRecordedTransport(clean), Cache: cache})
	for i := 0; i < 5; i++ {
		_, _ = guard.CheckInput(ctx, fmt.Sprintf("message %d", i), nil)
	}
	if cache.Len() != 2 {
		t.Fatalf("len=%d", cache.Len())
	}
}

func TestConversationsAreNotCachedByDefault(t *testing.T) {
	cache := NewLRUCache(0, 0)
	_, _ = New(Options{Transport: NewRecordedTransport(clean), Cache: cache}).CheckConversation(ctx, []Turn{{"user", "hi"}}, nil)
	if cache.Len() != 0 {
		t.Fatal("conversation cached")
	}
}

func TestTheGuardIsSafeForConcurrentUse(t *testing.T) {
	guard := New(Options{Transport: NewRecordedTransport(clean), Cache: NewLRUCache(0, 0)})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = guard.CheckInput(ctx, fmt.Sprintf("message %d", i%4), nil)
		}(i)
	}
	wg.Wait()
}

// -- prefilter ------------------------------------------------------------------

func TestPrefilterSettlesWithoutCallingJev(t *testing.T) {
	transport := NewRecordedTransport(clean)
	guard := New(Options{Transport: transport, Prefilter: PatternPrefilter{Patterns: CommonPatterns}})
	v, _ := guard.CheckOutput(ctx, "khoá của bạn là sk-ABCDEFGHIJKLMNOPQRSTUVWX", nil)
	if v.Action != Block || v.Prefilter != "openai-style-key" {
		t.Fatalf("got %+v", v)
	}
	if len(transport.Calls()) != 0 {
		t.Fatal("the deterministic case must not cost a round trip")
	}
}

func TestPrefilterLetsOrdinaryContentThrough(t *testing.T) {
	transport := NewRecordedTransport(clean)
	guard := New(Options{Transport: transport, Prefilter: PatternPrefilter{Patterns: CommonPatterns}})
	v, _ := guard.CheckInput(ctx, "xin chào, cho hỏi giờ mở cửa", nil)
	if v.Action != Allow || len(transport.Calls()) != 1 {
		t.Fatalf("got %+v", v)
	}
}

func TestPrefilterReadsConversationTurns(t *testing.T) {
	v, ok := PatternPrefilter{Patterns: CommonPatterns}.Decide(bundled(t), SurfaceConversation,
		ConversationState([]Turn{{"user", "hi"}, {"assistant", "-----BEGIN RSA PRIVATE KEY-----"}}, nil))
	if !ok || v.Prefilter != "private-key-block" {
		t.Fatalf("got %v %+v", ok, v)
	}
}

// -- observer -------------------------------------------------------------------

func TestObserverSeesEveryVerdict(t *testing.T) {
	var seen []string
	guard := New(Options{
		Transport: NewRecordedTransport(clean),
		Cache:     NewLRUCache(0, 0),
		Observer: func(v Verdict) {
			if v.Cached {
				seen = append(seen, "cached")
			} else {
				seen = append(seen, "fresh")
			}
		},
	})
	_, _ = guard.CheckInput(ctx, "same", nil)
	_, _ = guard.CheckInput(ctx, "same", nil)
	if strings.Join(seen, ",") != "fresh,cached" {
		t.Fatalf("got %v", seen)
	}
}

// -- session --------------------------------------------------------------------

func escalating(p *Policy) Verdict {
	return decideOn(p, SurfaceConversation, answers(A{"hazard": hazard(P{"ncr": 0.32}), "crescendo": noul(0.8), "escalation": score(3.4)}))
}

func TestAFlaggedConversationRaisesTheFloorForLaterTurns(t *testing.T) {
	p := bundled(t)
	session := NewSession("c1")
	session.Observe(escalating(p))
	if session.Floor() != Review {
		t.Fatalf("floor=%s", session.Floor())
	}
	v, _ := New(Options{Policy: p, Transport: NewRecordedTransport(clean)}).CheckInput(ctx, "một câu hỏi bình thường", &CheckOptions{Session: session})
	if v.Action != Review || !strings.HasPrefix(v.AppliedRules[len(v.AppliedRules)-1], "session-floor") {
		t.Fatalf("a clean turn inside an escalating conversation still holds: %+v", v)
	}
}

func TestTheFloorExpires(t *testing.T) {
	p := bundled(t)
	session := NewSession("c2")
	session.CarryTurns = 1
	session.Observe(decideOn(p, SurfaceConversation, answers(A{"hazard": hazard(P{"vcr": 0.9}), "actionability": score(2)})))
	if session.Floor() != Review {
		t.Fatalf("floor=%s", session.Floor())
	}
	session.Advance()
	if session.Floor() != Allow {
		t.Fatalf("floor=%s", session.Floor())
	}
}

func TestRiskDecays(t *testing.T) {
	p := bundled(t)
	session := NewSession("")
	session.Observe(decideOn(p, SurfaceInput, answers(A{"hazard": hazard(P{"vcr": 0.9}), "actionability": score(2)})))
	if !approx(session.Risk, 1) {
		t.Fatalf("risk=%v", session.Risk)
	}
	for i := 0; i < 3; i++ {
		session.Observe(decideOn(p, SurfaceInput, answers(nil)))
	}
	if session.Risk >= 0.2 {
		t.Fatalf("risk=%v", session.Risk)
	}
}

func TestTheDefaultTranscriptWindowIsTenTurns(t *testing.T) {
	// A default that drifts changes what every conversation check sees, silently.
	if NewSession("").MaxTurns != 10 || SessionFromState(map[string]any{}).MaxTurns != 10 {
		t.Fatal("default window is not ten")
	}
	session := NewSession("")
	for i := 0; i < 14; i++ {
		session.AddTurn("user", fmt.Sprintf("turn %d", i))
	}
	h := session.History()
	if len(h) != 10 || h[0].Content != "turn 4" {
		t.Fatalf("the window keeps the recent end: %v", h)
	}
}

func TestASessionSurvivesARoundTripThroughAStore(t *testing.T) {
	// A restored session must decide exactly as the one that was stored would have.
	p := bundled(t)
	session := NewSession("c4")
	session.AddTurn("user", "một câu hỏi")
	session.AddTurn("assistant", "một câu trả lời")
	session.Observe(escalating(p))

	encoded, _ := json.Marshal(session.AsState())
	var stored map[string]any
	_ = json.Unmarshal(encoded, &stored)
	restored := SessionFromState(stored)

	if restored.ID != session.ID || restored.Risk != session.Risk || restored.Floor() != Review {
		t.Fatalf("got %+v", restored.Summary())
	}
	if fmt.Sprint(restored.History()) != fmt.Sprint(session.History()) {
		t.Fatal("transcript lost")
	}
	v, _ := New(Options{Policy: p, Transport: NewRecordedTransport(clean)}).CheckInput(ctx, "một câu hỏi bình thường", &CheckOptions{Session: restored})
	if v.Action != Review {
		t.Fatalf("got %+v", v)
	}
	// And it expires on the same schedule, rather than resetting to two fresh turns.
	restored.Advance()
	restored.Advance()
	if restored.Floor() != Allow {
		t.Fatal("floor did not expire")
	}
}

func TestASessionWrittenByPythonReadsBack(t *testing.T) {
	// The keys are shared across languages; this is the shape Session.as_state() writes.
	stored := `{"id": "py", "decay": 0.5, "carry_turns": 2, "max_turns": 10,
		"turns": [{"role": "user", "content": "xin chào"}], "risk": 0.6, "floor": "review", "floor_turns_left": 1}`
	var state map[string]any
	_ = json.Unmarshal([]byte(stored), &state)
	s := SessionFromState(state)
	if s.ID != "py" || s.Floor() != Review || s.FloorTurnsLeft() != 1 || s.History()[0].Content != "xin chào" {
		t.Fatalf("got %+v", s.Summary())
	}
}

func TestAStoreRoundTripDropsAnExpiredFloor(t *testing.T) {
	session := NewSession("c5")
	session.CarryTurns = 1
	session.Observe(decideOn(bundled(t), SurfaceConversation, answers(A{"hazard": hazard(P{"vcr": 0.9}), "actionability": score(2)})))
	session.Advance()
	if session.Floor() != Allow || SessionFromState(session.AsState()).Floor() != Allow {
		t.Fatal("expired floor came back")
	}
}

func TestFromStateToleratesAHalfWrittenRecord(t *testing.T) {
	// A store can hand back junk. That should cost the conversation, not the request.
	cases := []map[string]any{
		{},
		{"floor": "review"},
		{"floor": "nonsense", "floor_turns_left": 5.0},
	}
	for _, c := range cases {
		if f := SessionFromState(c).Floor(); f != Allow {
			t.Fatalf("%v -> %s", c, f)
		}
	}
	s := SessionFromState(map[string]any{"turns": []any{map[string]any{"role": "user", "content": "hi"}}})
	if s.History()[0].Content != "hi" {
		t.Fatal("turn lost")
	}
}

func TestADegradedVerdictDoesNotMoveTheSession(t *testing.T) {
	session := NewSession("")
	session.Observe(ErrorVerdict(bundled(t), SurfaceOutput, &Error{Msg: "down"}, 0))
	if session.Risk != 0 || session.Floor() != Allow {
		t.Fatal("outage moved the session")
	}
}

func TestSessionMetadataReachesTheModel(t *testing.T) {
	transport := NewRecordedTransport(clean)
	session := NewSession("c3")
	session.AddTurn("user", "earlier")
	_, _ = New(Options{Transport: transport}).CheckInput(ctx, "now", &CheckOptions{Session: session})
	dc := transport.Calls()[0].State["deployment_context"].(map[string]any)
	if dc["conversation_id"] != "c3" || dc["turn_number"] != 2 {
		t.Fatalf("got %v", dc)
	}
}

func TestWithFloorRecomputesTheRoute(t *testing.T) {
	p := bundled(t)
	v := decideOn(p, SurfaceOutput, answers(A{"hazard": hazard(P{"prv": 0.2}), "refusal": noul(0)}))
	if v.Action != Flag {
		t.Fatalf("got %s", v.Action)
	}
	before := len(v.AppliedRules)
	raised := WithFloor(p, v, Review, "test")
	if raised.Action != Review || raised.Route != RouteRedact {
		t.Fatalf("the hazard still decides the handling: %+v", raised)
	}
	if len(v.AppliedRules) != before || v.Action != Flag {
		t.Fatal("WithFloor changed the original verdict")
	}
}

func TestCheckTurnAddsTheConversationWhenThereIsHistory(t *testing.T) {
	guard := New(Options{Transport: NewRecordedTransport(clean)})
	without, _ := guard.CheckTurn(ctx, "hi", "hello", nil)
	with, _ := guard.CheckTurn(ctx, "hi", "hello", &CheckOptions{History: []Turn{{"user", "earlier"}}})
	if len(without) != 2 || len(with) != 3 {
		t.Fatalf("without=%d with=%d", len(without), len(with))
	}
}

// -- quick checks ---------------------------------------------------------------

func TestQuickChecksAskOnlyTheSentinels(t *testing.T) {
	questions, err := BuildQuestions(bundled(t), SurfaceOutput, false, SubsetSentinels)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name, q := range questions {
		names = append(names, name)
		if q.Type != "noul" {
			t.Fatalf("%s is %s", name, q.Type)
		}
	}
	for _, want := range []string{"s_cse", "s_iwp", "s_prv", "s_sid", "s_ssh", "s_vcr"} {
		if !contains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	if len(names) != 6 {
		t.Fatalf("got %v; the 18-label choice is the expensive part to skip", names)
	}
}

func TestQuickVerdictsAreMarkedPartial(t *testing.T) {
	transport := NewRecordedTransport(Answers{"s_sid": noul(0.05), "s_prv": noul(0.05)})
	v, _ := New(Options{Transport: transport}).CheckOutput(ctx, "một phần câu trả lời", &CheckOptions{Quick: true})
	if !v.Partial || v.Action != Allow {
		t.Fatalf("got %+v", v)
	}
}

// -- streaming ------------------------------------------------------------------

// scripted answers each call from a list, so a stream can be steered chunk by chunk.
type scripted struct {
	mu      sync.Mutex
	answers []Answers
	calls   int
}

func (s *scripted) SystemOne(context.Context, State, Questions, CallOptions) (Reply, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.answers[min(s.calls, len(s.answers)-1)]
	s.calls++
	return Reply{Answers: a, Model: "scripted"}, nil
}

func sentinels(sid float64) Answers {
	out := Answers{}
	for _, c := range []string{"cse", "iwp", "prv", "sid", "ssh", "vcr"} {
		out["s_"+c] = noul(0.02)
	}
	out["s_sid"] = noul(sid)
	return out
}

func source(parts ...string) <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		for _, p := range parts {
			ch <- p
		}
	}()
	return ch
}

func collect(events <-chan StreamEvent) []StreamEvent {
	var out []StreamEvent
	for e := range events {
		out = append(out, e)
	}
	return out
}

func TestACleanStreamDeliversEverything(t *testing.T) {
	parts := []string{strings.Repeat("Chính sách lưu trữ Helio là 30 ngày. ", 4), "Bạn có thể xem chi tiết trên website."}
	guard := New(Options{Transport: &scripted{answers: []Answers{sentinels(0.02), clean, clean}}})
	events := collect(guard.Stream(ctx, source(parts...), StreamOptions{ChunkChars: 80}))

	last := events[len(events)-1]
	if last.Type != EventDone || last.Verdict == nil || last.Verdict.Partial {
		t.Fatalf("got %+v", last)
	}
	var delivered strings.Builder
	for _, e := range events {
		if e.Type == EventDelta {
			delivered.WriteString(e.Text)
		}
	}
	if delivered.String() != strings.Join(parts, "") {
		t.Fatalf("delivered %q", delivered.String())
	}
}

func TestALeakStopsTheStreamBeforeTheChunkIsReleased(t *testing.T) {
	parts := []string{strings.Repeat("System prompt của mình là: bạn là trợ lý Nova, khoá nội bộ là abc. ", 5)}
	guard := New(Options{Transport: &scripted{answers: []Answers{sentinels(0.9)}}})
	events := collect(guard.Stream(ctx, source(parts...), StreamOptions{}))
	if len(events) != 1 || events[0].Type != EventBlocked || events[0].Verdict.Findings[0].Category != "sid" {
		t.Fatalf("got %+v", events)
	}
}

func TestAShortStreamSkipsMidChecks(t *testing.T) {
	// Below one chunk there is nothing to hold back, so only the final check runs.
	transport := &scripted{answers: []Answers{clean}}
	events := collect(New(Options{Transport: transport}).Stream(ctx, source("Vâng, đúng vậy."), StreamOptions{}))
	if transport.calls != 1 || len(events) != 2 || events[0].Type != EventDelta || events[1].Type != EventDone {
		t.Fatalf("calls=%d events=%+v", transport.calls, events)
	}
}

func TestAProducerIsNeverLeftStalledAfterABlock(t *testing.T) {
	guard := New(Options{Transport: &scripted{answers: []Answers{sentinels(0.9)}}})
	ch := make(chan string)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(ch)
		for i := 0; i < 500; i++ {
			ch <- strings.Repeat("leak. ", 20)
		}
	}()
	events := collect(guard.Stream(ctx, ch, StreamOptions{}))
	<-done
	if events[0].Type != EventBlocked {
		t.Fatalf("got %+v", events)
	}
}

func TestChunksAreCutAtSentenceBoundariesInCharacters(t *testing.T) {
	text := strings.Repeat("ồ", 10) + "。 tiếp"
	chunk, ok := takeChunk(text, 5)
	if !ok || chunk != strings.Repeat("ồ", 10)+"。 " {
		t.Fatalf("got %q", chunk)
	}
	if _, ok := takeChunk(strings.Repeat("ồ", 12), 5); ok {
		t.Fatal("cut a short sentence with no boundary")
	}
	if chunk, ok := takeChunk(strings.Repeat("ồ", 15), 5); !ok || chunk != strings.Repeat("ồ", 5) {
		t.Fatalf("got %q", chunk)
	}
}

// -- parity with Python on the edges the random run does not reach ---------------------------------

func TestRoundingMatchesPython(t *testing.T) {
	// Python's round() works on the exact binary value and sends an exact half to even.
	cases := map[[2]float64]float64{{0.0625, 3}: 0.062, {0.125, 2}: 0.12, {2.675, 2}: 2.67, {2.5, 0}: 2}
	for in, want := range cases {
		if got := round(in[0], int(in[1])); got != want {
			t.Fatalf("round(%v, %v) = %v, want %v", in[0], in[1], got, want)
		}
	}
	s := NewSession("r")
	s.Observe(Verdict{Action: Block, Surface: SurfaceInput})
	for i := 0; i < 4; i++ {
		s.Observe(Verdict{Action: Allow, Surface: SurfaceInput})
	}
	if got := s.Metadata()["session_risk"]; got != 0.062 {
		t.Fatalf("session_risk = %v, want Python's 0.062", got)
	}
}

func TestWholeNumbersFromJSONReadAsPythonInts(t *testing.T) {
	var pack map[string]any
	data, _ := bundledPacks.ReadFile("policies/standard-v1.json")
	_ = json.Unmarshal(data, &pack)
	pack["version"] = 2.0
	p, err := NewPolicy(pack)
	if err != nil {
		t.Fatal(err)
	}
	if p.QualifiedID() != "standard-v1@2" {
		t.Fatalf("got %s", p.QualifiedID())
	}
	if r := RecordFromJSON(map[string]any{"id": 17.0}, ""); r.ID != "17" {
		t.Fatalf("got %q", r.ID)
	}
}

func TestPolicyFlagsReadAsPythonTruthiness(t *testing.T) {
	var pack map[string]any
	data, _ := bundledPacks.ReadFile("policies/standard-v1.json")
	_ = json.Unmarshal(data, &pack)
	cats := pack["categories"].(map[string]any)
	cats["hte"].(map[string]any)["enabled"] = nil
	cats["ncr"].(map[string]any)["enabled"] = 0.0
	cats["vcr"].(map[string]any)["sentinel"] = 0.0
	cats["elc"].(map[string]any)["sentinel"] = 1.0
	cats["elc"].(map[string]any)["sentinel_instructions"] = "x"
	pack["defaults"].(map[string]any)["on_error"] = nil
	p, err := NewPolicy(pack)
	if err != nil {
		t.Fatal(err)
	}
	if p.Categories["hte"].Enabled || p.Categories["ncr"].Enabled || p.Categories["vcr"].Sentinel || !p.Categories["elc"].Sentinel {
		t.Fatal("flags not read as Python's bool()")
	}
	if p.FailClosed(SurfaceOutput) {
		t.Fatal("a null on_error fails open in Python")
	}
}

func TestPrefilterBoundariesAreUnicodeAware(t *testing.T) {
	pf := PatternPrefilter{Patterns: CommonPatterns}
	p := bundled(t)
	for _, text := range []string{"số012345678901", "012345678901đ"} {
		if _, ok := pf.Decide(p, SurfaceInput, InputState(text, nil)); ok {
			t.Fatalf("%q matched; Python's \\b sees no boundary there", text)
		}
	}
	for _, text := range []string{"CCCD: 012345678901.", "012345678901"} {
		if _, ok := pf.Decide(p, SurfaceInput, InputState(text, nil)); !ok {
			t.Fatalf("%q did not match", text)
		}
	}
}

func TestCheckTurnFallsBackToTheSessionOnAnEmptyHistory(t *testing.T) {
	session := NewSession("h")
	session.AddTurn("user", "earlier")
	got, _ := New(Options{Transport: NewRecordedTransport(clean)}).CheckTurn(ctx, "hi", "hello", &CheckOptions{Session: session, History: []Turn{}})
	if _, ok := got[SurfaceConversation]; !ok {
		t.Fatal("an empty history did not fall back to the session's")
	}
}
