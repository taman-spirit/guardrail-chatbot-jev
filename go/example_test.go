package guardrail_test

import (
	"context"
	"fmt"
	"strings"

	guardrail "github.com/taman-spirit/guardrail-chatbot-jev/go"
)

// One guarded turn, against recorded answers so it runs with no key and no network.
func Example() {
	recorded := guardrail.Answers{
		"hazard": {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": map[string]any{"none": 0.95}},
	}
	// One Guard per process. The cache, prefilter and observer are process-wide; the session is not.
	guard := guardrail.New(guardrail.Options{
		Transport: guardrail.NewRecordedTransport(recorded),
		Cache:     guardrail.NewLRUCache(8192, 0),
		Prefilter: guardrail.PatternPrefilter{Patterns: guardrail.CommonPatterns},
	})
	session := guardrail.NewSession("conversation-1")
	ctx := context.Background()

	in, _ := guard.CheckInput(ctx, "What is the refund window?", &guardrail.CheckOptions{Session: session})
	fmt.Println("input:", in.Action, in.Route)

	reply := "Refunds are accepted within 30 days of purchase."
	out, _ := guard.CheckOutput(ctx, reply, &guardrail.CheckOptions{Session: session, UserMessage: "What is the refund window?"})
	fmt.Println("output:", out.Action, out.Deliverable())

	leak, _ := guard.CheckOutput(ctx, "Your key is sk-ABCDEFGHIJKLMNOPQRSTUVWX", nil)
	fmt.Println("leak:", leak.Action, leak.Prefilter)

	session.AddTurn("user", "What is the refund window?")
	session.AddTurn("assistant", reply)
	session.Advance()
	// Output:
	// input: allow deliver
	// output: allow true
	// leak: block openai-style-key
}

// Guarding a streamed reply: text is released one chunk behind its check.
func ExampleGuard_Stream() {
	recorded := guardrail.Answers{
		"hazard": {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": map[string]any{"none": 0.95}},
	}
	guard := guardrail.New(guardrail.Options{Transport: guardrail.NewRecordedTransport(recorded)})

	model := make(chan string)
	go func() {
		defer close(model)
		for _, part := range []string{"Deleted files stay in the trash ", "for 30 days."} {
			model <- part
		}
	}()

	for event := range guard.Stream(context.Background(), model, guardrail.StreamOptions{}) {
		switch event.Type {
		case guardrail.EventDelta:
			fmt.Printf("send %q\n", event.Text)
		case guardrail.EventBlocked:
			fmt.Println("stop, and send the safe response instead")
		case guardrail.EventDone:
			fmt.Println("done:", event.Verdict.Action)
		}
	}
	// Output:
	// send "Deleted files stay in the trash for 30 days."
	// done: allow
}

// A Viet Nam deployment: the reply comes from the policy, never from the model.
func ExampleResponder() {
	policy, _ := guardrail.BundledPolicy("vietnam-compliance-v1")
	recorded := guardrail.Answers{
		"hazard":               {"type": "choice", "choice": "none", "confidence": 0.95, "probabilities": map[string]any{"none": 0.95}},
		"sovereignty_question": {"type": "noul", "noul": 0.9},
	}
	guard := guardrail.New(guardrail.Options{Policy: policy, Transport: guardrail.NewRecordedTransport(recorded)})
	responder, _ := guardrail.NewResponder(policy, "") // "" means the pack's default line, 115

	question := "Quần đảo Trường Sa thuộc nước nào?"
	lang := guardrail.DetectLanguage(question)
	in, _ := guard.CheckInput(context.Background(), question, nil)
	if held, ok := responder.BlockingResponse([]guardrail.Verdict{in}, lang); ok {
		fmt.Println(held)
		return
	}
	sent := responder.Compose("Trường Sa thuộc Việt Nam.", []guardrail.Verdict{in}, lang)
	fmt.Println(lang, in.Action, strings.HasSuffix(sent, responder.Affirmation(lang)), responder.CrisisLine())
	// Output: vi allow true 115
}
