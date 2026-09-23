package guardrail

import (
	"context"
	"unicode"
)

// EventType says what happened while a streamed reply was being guarded.
type EventType string

const (
	// EventDelta carries text cleared for delivery.
	EventDelta EventType = "delta"
	// EventBlocked means the stream stopped and nothing further should be sent.
	EventBlocked EventType = "blocked"
	// EventDone carries the verdict on the complete reply. It is always last unless blocked.
	EventDone EventType = "done"
	// EventError carries a check that failed with RaiseOnError set.
	EventError EventType = "error"
)

// StreamEvent is one thing that happened while the reply was being guarded.
type StreamEvent struct {
	Type    EventType
	Text    string
	Verdict *Verdict
	Err     error
}

// StreamOptions configures Guard.Stream.
type StreamOptions struct {
	UserMessage string
	Context     []string
	Session     *Session
	Metadata    map[string]any
	// ChunkChars is the minimum number of characters before looking for a sentence boundary to
	// cut at; it defaults to 280. Smaller means more round trips and a tighter hold; larger means
	// fewer, coarser checks.
	ChunkChars int
}

// boundaries end a sentence, including the Vietnamese and CJK forms.
var boundaries = map[rune]bool{'.': true, '!': true, '?': true, ';': true, ':': true, '\n': true, '。': true, '！': true, '？': true}

// Stream guards a streamed reply, releasing text one chunk behind its check.
//
// A streamed reply cannot be checked before its first token, and a check that waits for the last
// one gives up streaming altogether. The middle path is to release the text one chunk behind:
// chunk k is held until its check returns, while the model is already producing chunk k+1. Only
// the first chunk pays the full latency.
//
// Mid-stream checks ask the sentinel questions only: the categories where a miss is unacceptable,
// which are also the ones judgeable from partial text. The complete reply gets the full question
// set at the end.
//
// Close source when the model is done. Read the returned channel until it closes; stop sending
// on a blocked event, and cancel the model call. The guard keeps draining source after a block
// so the producer never stalls. If ctx is cancelled the channel closes without a done event.
func (g *Guard) Stream(ctx context.Context, source <-chan string, opts StreamOptions) <-chan StreamEvent {
	chunkChars := opts.ChunkChars
	if chunkChars <= 0 {
		chunkChars = 280
	}
	out := make(chan StreamEvent)

	// Buffer the source, so the model keeps writing while a check is in flight.
	buffered := make(chan string, 64)
	stop := make(chan struct{})
	go func() {
		defer close(buffered)
		stopped := false
		for delta := range source {
			if stopped {
				continue
			}
			select {
			case buffered <- delta:
			case <-stop:
				stopped = true
			}
		}
	}()

	go func() {
		defer close(out)
		defer close(stop)

		emit := func(e StreamEvent) bool {
			select {
			case out <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		check := func(text string, quick bool) (Verdict, bool) {
			v, err := g.CheckOutput(ctx, text, &CheckOptions{
				UserMessage: opts.UserMessage,
				Context:     opts.Context,
				Metadata:    opts.Metadata,
				Session:     opts.Session,
				Quick:       quick,
			})
			if err != nil {
				emit(StreamEvent{Type: EventError, Err: err})
				return v, false
			}
			if !v.Deliverable() {
				emit(StreamEvent{Type: EventBlocked, Verdict: &v})
				return v, false
			}
			return v, true
		}

		delivered, pending := "", ""
		for {
			var delta string
			var open bool
			select {
			case delta, open = <-buffered:
			case <-ctx.Done():
				return
			}
			if !open {
				break
			}
			pending += delta
			chunk, ok := takeChunk(pending, chunkChars)
			if !ok {
				continue
			}
			pending = pending[len(chunk):]
			if _, ok := check(delivered+chunk, true); !ok {
				return
			}
			delivered += chunk
			if !emit(StreamEvent{Type: EventDelta, Text: chunk}) {
				return
			}
		}

		final := delivered + pending
		verdict, ok := check(final, false)
		if !ok {
			return
		}
		if pending != "" && !emit(StreamEvent{Type: EventDelta, Text: pending}) {
			return
		}
		emit(StreamEvent{Type: EventDone, Text: final, Verdict: &verdict})
	}()
	return out
}

// takeChunk returns the next releasable chunk, cut at a sentence boundary once past chunkChars.
// Lengths are in characters, not bytes, so a Vietnamese reply is cut where an English one would be.
func takeChunk(pending string, chunkChars int) (string, bool) {
	runes := []rune(pending)
	if len(runes) < chunkChars {
		return "", false
	}
	for i := max(chunkChars, 1); i < len(runes); i++ {
		if unicode.IsSpace(runes[i]) && boundaries[runes[i-1]] {
			return string(runes[:i+1]), true
		}
	}
	// No boundary in sight and the buffer is getting long: cut anyway rather than hold the stream
	// hostage to a model that is writing one very long sentence.
	if len(runes) >= chunkChars*3 {
		return string(runes[:chunkChars]), true
	}
	return "", false
}
