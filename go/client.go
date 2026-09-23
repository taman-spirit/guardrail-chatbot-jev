package guardrail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is where Jev is served. This is a default, not a constraint: set JEV_BASE_URL or
// HTTPOptions.BaseURL to point at a gateway or a proxy in front of it.
const DefaultBaseURL = "https://api.typesafe.ai"

// DefaultModel is the Jev model used when none is configured.
const DefaultModel = "jev-latest"

const endpoint = "/v1/systemone"

var retryStatuses = map[int]bool{408: true, 429: true, 500: true, 502: true, 503: true, 504: true, 529: true}

// CallOptions are per-call overrides.
type CallOptions struct {
	// Model overrides the transport's model for this call.
	Model string
	// Timeout overrides the transport's per-attempt timeout for this call.
	Timeout time.Duration
}

// Reply is what Jev sent back for one question set.
type Reply struct {
	Answers Answers
	Model   string
	Usage   Usage
}

// Transport is anything that can answer a question set. Implement it to reach Jev through your
// provider's own SDK, a gateway, or a test double.
type Transport interface {
	SystemOne(ctx context.Context, state State, questions Questions, opts CallOptions) (Reply, error)
}

// HTTPOptions configures an HTTPTransport. Zero values take the defaults.
type HTTPOptions struct {
	// APIKey defaults to JEV_API_KEY.
	APIKey string
	// BaseURL defaults to JEV_BASE_URL, then DefaultBaseURL.
	BaseURL string
	// Model defaults to JEV_MODEL, then DefaultModel.
	Model string
	// Timeout is per attempt; it defaults to 10s.
	Timeout time.Duration
	// MaxRetries is the number of retries after the first attempt. Zero means the default of 2;
	// a negative value means no retries.
	MaxRetries int
	// BackoffInitial defaults to 500ms, BackoffMax to 5s.
	BackoffInitial time.Duration
	BackoffMax     time.Duration
	// Client defaults to a fresh http.Client.
	Client *http.Client
}

// HTTPTransport is the zero-dependency transport over POST /v1/systemone.
type HTTPTransport struct {
	APIKey         string
	BaseURL        string
	Model          string
	Timeout        time.Duration
	MaxRetries     int
	BackoffInitial time.Duration
	BackoffMax     time.Duration
	Client         *http.Client
}

// NewHTTPTransport builds the default transport. It fails when there is no API key.
func NewHTTPTransport(opts HTTPOptions) (*HTTPTransport, error) {
	key := strings.TrimSpace(firstNonEmpty(opts.APIKey, os.Getenv("JEV_API_KEY")))
	if key == "" {
		return nil, &Error{Msg: "No Jev API key. Set JEV_API_KEY, or pass HTTPOptions.APIKey, " +
			"or use a recorded transport for offline work."}
	}
	t := &HTTPTransport{
		APIKey:         key,
		BaseURL:        strings.TrimRight(strings.TrimSpace(firstNonEmpty(opts.BaseURL, os.Getenv("JEV_BASE_URL"), DefaultBaseURL)), "/"),
		Model:          firstNonEmpty(opts.Model, os.Getenv("JEV_MODEL"), DefaultModel),
		Timeout:        orDuration(opts.Timeout, 10*time.Second),
		MaxRetries:     opts.MaxRetries,
		BackoffInitial: orDuration(opts.BackoffInitial, 500*time.Millisecond),
		BackoffMax:     orDuration(opts.BackoffMax, 5*time.Second),
		Client:         opts.Client,
	}
	switch {
	case t.MaxRetries == 0:
		t.MaxRetries = 2
	case t.MaxRetries < 0:
		t.MaxRetries = 0
	}
	if t.Client == nil {
		t.Client = &http.Client{}
	}
	return t, nil
}

// SystemOne sends one question set, retrying transient failures with jittered backoff.
func (t *HTTPTransport) SystemOne(ctx context.Context, state State, questions Questions, opts CallOptions) (Reply, error) {
	body, err := json.Marshal(map[string]any{
		"model":     firstNonEmpty(opts.Model, t.Model),
		"state":     state,
		"questions": questions,
	})
	if err != nil {
		return Reply{}, errorf(err, "cannot encode the Jev request: %v", err)
	}
	timeout := orDuration(opts.Timeout, t.Timeout)

	delay := t.BackoffInitial
	var last *Error
	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		reply, status, retryAfter, err := t.attempt(ctx, body, timeout)
		if err == nil {
			return reply, nil
		}
		last = err
		if ctx.Err() != nil || attempt == t.MaxRetries {
			break
		}
		if status != 0 {
			if !retryStatuses[status] {
				break
			}
			if retryAfter >= 0 {
				delay = min(retryAfter, t.BackoffMax)
			}
		}
		sleep := time.Duration(float64(delay) * (1 - rand.Float64()*0.25))
		select {
		case <-ctx.Done():
			return Reply{}, errorf(ctx.Err(), "Jev API unreachable: %v", ctx.Err())
		case <-time.After(sleep):
		}
		delay = min(delay*2, t.BackoffMax)
	}
	if last == nil {
		last = &Error{Msg: "Jev API call failed"}
	}
	return Reply{}, last
}

// attempt makes one request. A non-zero status means the server answered with an error.
func (t *HTTPTransport) attempt(ctx context.Context, body []byte, timeout time.Duration) (Reply, int, time.Duration, *Error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.BaseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return Reply{}, 0, -1, errorf(err, "Jev API unreachable: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "guardrail-chatbot-jev/1.0 (+go)")

	resp, err := t.Client.Do(req)
	if err != nil {
		return Reply{}, 0, -1, errorf(err, "Jev API unreachable: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		retryAfter := time.Duration(-1)
		if secs, perr := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64); perr == nil && secs >= 0 {
			retryAfter = time.Duration(secs * float64(time.Second))
		}
		return Reply{}, resp.StatusCode, retryAfter,
			&Error{Msg: fmt.Sprintf("Jev API returned %d: %q", resp.StatusCode, snippet)}
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Reply{}, 0, -1, errorf(err, "Jev API unreachable: %v", err)
	}
	return unpack(payload), 0, -1, nil
}

func unpack(payload map[string]any) Reply {
	answers := Answers{}
	for name, a := range mapOf(payload["answers"]) {
		answers[name] = Answer(mapOf(a))
	}
	usage := mapOf(payload["usage"])
	return Reply{
		Answers: answers,
		Model:   stringOr(payload["model"], ""),
		Usage: Usage{
			InputTokens:  intOr(usage["input_tokens"], 0),
			OutputTokens: intOr(usage["output_tokens"], 0),
		},
	}
}

// RecordedCall is one request a RecordedTransport received.
type RecordedCall struct {
	State     State
	Questions Questions
}

// RecordedTransport replays answers recorded earlier. For tests, offline work and policy tuning.
type RecordedTransport struct {
	Answers Answers
	Model   string

	mu    sync.Mutex
	calls []RecordedCall
}

// NewRecordedTransport replays the same answers for every call.
func NewRecordedTransport(answers Answers) *RecordedTransport {
	return &RecordedTransport{Answers: answers, Model: "recorded"}
}

// SystemOne returns the recorded answers.
func (t *RecordedTransport) SystemOne(_ context.Context, state State, questions Questions, _ CallOptions) (Reply, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, RecordedCall{State: state, Questions: questions})
	return Reply{Answers: t.Answers, Model: t.Model}, nil
}

// Calls lists every request received so far.
func (t *RecordedTransport) Calls() []RecordedCall {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]RecordedCall(nil), t.calls...)
}

// Exchange is one recorded request and its answers.
type Exchange struct {
	State   State   `json:"state"`
	Answers Answers `json:"answers"`
	Model   string  `json:"model"`
	Usage   Usage   `json:"usage"`
}

// RecordingTransport wraps another transport and keeps every exchange.
//
// A calibration run is expensive: it costs tokens, it costs rate-limit budget, and the labelled
// set it runs against is the scarce thing. Recording the raw answers makes that one run
// replayable, so every later threshold change is an offline question instead of another call.
type RecordingTransport struct {
	Inner Transport

	mu      sync.Mutex
	records []Exchange
}

// SystemOne forwards to the inner transport and records the exchange.
func (t *RecordingTransport) SystemOne(ctx context.Context, state State, questions Questions, opts CallOptions) (Reply, error) {
	reply, err := t.Inner.SystemOne(ctx, state, questions, opts)
	if err != nil {
		return reply, err
	}
	t.mu.Lock()
	t.records = append(t.records, Exchange{State: state, Answers: reply.Answers, Model: reply.Model, Usage: reply.Usage})
	t.mu.Unlock()
	return reply, nil
}

// Records lists every exchange so far.
func (t *RecordingTransport) Records() []Exchange {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Exchange(nil), t.records...)
}

// TransportFunc adapts a function to the Transport interface.
type TransportFunc func(ctx context.Context, state State, questions Questions, opts CallOptions) (Reply, error)

// SystemOne calls f.
func (f TransportFunc) SystemOne(ctx context.Context, state State, questions Questions, opts CallOptions) (Reply, error) {
	return f(ctx, state, questions, opts)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func orDuration(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}
