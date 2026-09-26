//go:build live

package guardrail

// A live multi-turn run against Jev. It needs JEV_API_KEY, so it is behind the "live" build tag and
// never runs in CI:
//
//	JEV_API_KEY=... LIVE_OUT=/tmp/multiturn go test -tags live -run TestLiveMultiturn -v ./
//
// Every case in examples/multiturn-live.jsonl is played as a whole conversation: the earlier turns
// go through Jev too, so the session state forms the way it would in production, and only then is
// the latest turn checked. Each case runs twice, under MultiturnFloor and MultiturnAttribute. The
// harmful material comes from the labelled cases already in the repository, by reference.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

type liveCase struct {
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	Lang                string `json:"lang"`
	PriorInput          string `json:"prior_input"`
	PriorOutputRef      string `json:"prior_output_ref"`
	PriorReply          string `json:"prior_reply"`
	UserMessage         string `json:"user_message"`
	Reply               string `json:"reply"`
	ReplyRef            string `json:"reply_ref"`
	ConversationRef     string `json:"conversation_ref"`
	ExpectedWithheld    bool   `json:"expected_withheld"`
	ExpectedConvFlagged bool   `json:"expected_conversation_flagged"`
	Note                string `json:"note"`
}

type liveResult struct {
	ID          string       `json:"id"`
	Kind        string       `json:"kind"`
	Mode        string       `json:"mode"`
	PriorHeld   []bool       `json:"prior_held"`
	InputAction Action       `json:"input_action"`
	Output      *Verdict     `json:"output,omitempty"`
	Held        bool         `json:"held"`
	Stage       string       `json:"stage"`
	Context     *ContextRead `json:"context,omitempty"`
	ConvAction  Action       `json:"conversation_action"`
	Want        bool         `json:"want"`
	Correct     bool         `json:"correct"`
	Degraded    bool         `json:"degraded"`
}

func readJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var rows []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return rows
}

func byID(rows []map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, r := range rows {
		out[r["id"].(string)] = r
	}
	return out
}

func TestLiveMultiturn(t *testing.T) {
	if os.Getenv("JEV_API_KEY") == "" {
		t.Skip("JEV_API_KEY is not set")
	}
	inputs := byID(readJSONL(t, "../examples/cases-input.jsonl"))
	outputs := byID(readJSONL(t, "../examples/cases-output.jsonl"))
	convs := byID(readJSONL(t, "../examples/conversations.jsonl"))
	var cases []liveCase
	for _, row := range readJSONL(t, "../examples/multiturn-live.jsonl") {
		raw, _ := json.Marshal(row)
		var c liveCase
		_ = json.Unmarshal(raw, &c)
		cases = append(cases, c)
	}

	http, err := NewHTTPTransport(HTTPOptions{})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &RecordingTransport{Inner: http}
	p := bundled(t)

	type job struct {
		c    liveCase
		mode MultiturnMode
	}
	modeName := map[MultiturnMode]string{MultiturnFloor: "floor", MultiturnAttribute: "attribute"}
	var jobs []job
	for _, c := range cases {
		jobs = append(jobs, job{c, MultiturnFloor}, job{c, MultiturnAttribute})
	}

	results := make([]liveResult, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = playLive(t, p, recorder, j.c, j.mode, inputs, outputs, convs)
			results[i].Mode = modeName[j.mode]
		}(i, j)
	}
	wg.Wait()

	if dir := os.Getenv("LIVE_OUT"); dir != "" {
		_ = os.MkdirAll(dir, 0o700)
		writeJSONL(t, filepath.Join(dir, "results.jsonl"), results)
		writeJSONL(t, filepath.Join(dir, "answers.jsonl"), recorder.Records())
	}
	t.Log("\n" + liveReport(results))
}

func playLive(t *testing.T, p *Policy, transport Transport, c liveCase, mode MultiturnMode,
	inputs, outputs, convs map[string]map[string]any) liveResult {
	guard := New(Options{Policy: p, Transport: transport, Multiturn: mode})
	session := NewSession(c.ID)
	res := liveResult{ID: c.ID, Kind: c.Kind}

	// The earlier turns, as a list of (role, text).
	var prior [][2]string
	latest, reply := c.UserMessage, c.Reply
	switch {
	case c.ConversationRef != "":
		turns := convs[c.ConversationRef]["turns"].([]any)
		for _, x := range turns[:len(turns)-1] {
			m := x.(map[string]any)
			prior = append(prior, [2]string{m["role"].(string), m["content"].(string)})
		}
		latest = turns[len(turns)-1].(map[string]any)["content"].(string)
		reply = ""
		res.Want = c.ExpectedConvFlagged
	case c.PriorOutputRef != "":
		prior = [][2]string{{"user", outputs[c.PriorOutputRef]["user_message"].(string)}, {"assistant", c.PriorReply}}
		reply = outputs[c.ReplyRef]["text"].(string)
		res.Want = c.ExpectedWithheld
	case c.PriorInput != "":
		prior = [][2]string{{"user", inputs[c.PriorInput]["text"].(string)}, {"assistant", c.PriorReply}}
		res.Want = c.ExpectedWithheld
	default:
		res.Want = c.ExpectedWithheld
	}

	record := func(role, text string, v Verdict) {
		if mode == MultiturnAttribute {
			session.Record(role, text, v)
		} else {
			session.AddTurn(role, text) // the README's pattern before this change: verbatim
		}
	}
	lastUser := ""
	userHeld := false
	for _, turn := range prior {
		switch turn[0] {
		case "user":
			v, _ := guard.CheckInput(ctx, turn[1], &CheckOptions{Session: session})
			res.Degraded = res.Degraded || v.Degraded
			userHeld = !v.Deliverable()
			res.PriorHeld = append(res.PriorHeld, userHeld)
			record("user", turn[1], v)
			lastUser = turn[1]
		case "assistant":
			if userHeld {
				// The safe response went out instead; the model's turn never happened.
				session.Advance()
				_, _ = guard.CheckConversation(ctx, session.History(), &CheckOptions{Session: session})
				continue
			}
			v, _ := guard.CheckOutput(ctx, turn[1], &CheckOptions{Session: session, UserMessage: lastUser})
			res.Degraded = res.Degraded || v.Degraded
			res.PriorHeld = append(res.PriorHeld, !v.Deliverable())
			record("assistant", turn[1], v)
			session.Advance()
			_, _ = guard.CheckConversation(ctx, session.History(), &CheckOptions{Session: session})
		}
	}

	in, _ := guard.CheckInput(ctx, latest, &CheckOptions{Session: session})
	res.InputAction = in.Action
	res.Degraded = res.Degraded || in.Degraded
	switch {
	case !in.Deliverable():
		res.Held, res.Stage = true, "input"
	case reply != "":
		out, _ := guard.CheckOutput(ctx, reply, &CheckOptions{Session: session, UserMessage: latest})
		res.Output = &out
		res.Context = out.Context
		res.Degraded = res.Degraded || out.Degraded
		if !out.Deliverable() {
			res.Held, res.Stage = true, "output"
			if out.HasRule("context-attributed") {
				res.Stage = "context"
			}
		}
	}
	turns := append(session.History(), Turn{Role: "user", Content: latest})
	conv, _ := guard.CheckConversation(ctx, turns, nil)
	res.ConvAction = conv.Action

	if c.ConversationRef != "" {
		res.Correct = (Rank(conv.Action) >= Rank(Review)) == c.ExpectedConvFlagged
	} else {
		res.Correct = res.Held == c.ExpectedWithheld
	}
	return res
}

func liveReport(results []liveResult) string {
	var b strings.Builder
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Mode != results[j].Mode {
			return results[i].Mode < results[j].Mode
		}
		return results[i].ID < results[j].ID
	})
	type agg struct{ n, held, correct int }
	byKind := map[string]map[string]*agg{}
	degraded := 0
	for _, r := range results {
		if r.Degraded {
			degraded++
		}
		ctxNote := ""
		if r.Context != nil {
			ctxNote = fmt.Sprintf("ctx completes=%.2f disengages=%.2f attributed=%v %v", r.Context.Completes, r.Context.Disengages, r.Context.Attributed, r.Context.Categories)
		}
		out := "-"
		if r.Output != nil {
			out = fmt.Sprintf("%s %v", r.Output.Action, r.Output.Categories())
		}
		mark := "ok  "
		if !r.Correct {
			mark = "MISS"
		}
		fmt.Fprintf(&b, "%s %-9s %-28s want=%-5v held=%-5v %-7s in=%-6s out=%-22s conv=%-6s prior_held=%v %s\n",
			mark, r.Mode, r.ID, r.Want, r.Held, r.Stage, r.InputAction, out, r.ConvAction, r.PriorHeld, ctxNote)
		if byKind[r.Kind] == nil {
			byKind[r.Kind] = map[string]*agg{}
		}
		a := byKind[r.Kind][r.Mode]
		if a == nil {
			a = &agg{}
			byKind[r.Kind][r.Mode] = a
		}
		a.n++
		if r.Held {
			a.held++
		}
		if r.Correct {
			a.correct++
		}
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	fmt.Fprintf(&b, "\n%-22s %-26s %-26s\n", "kind", "floor: held / correct", "attribute: held / correct")
	for _, k := range kinds {
		f, a := byKind[k]["floor"], byKind[k]["attribute"]
		fmt.Fprintf(&b, "%-22s %2d/%-2d held, %2d/%-2d right   %2d/%-2d held, %2d/%-2d right\n", k, f.held, f.n, f.correct, f.n, a.held, a.n, a.correct, a.n)
	}
	fmt.Fprintf(&b, "\ndegraded (Jev unreachable during the case): %d of %d runs\n", degraded, len(results))
	return b.String()
}

func writeJSONL[T any](t *testing.T, path string, rows []T) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, r := range rows {
		_ = enc.Encode(r)
	}
}
