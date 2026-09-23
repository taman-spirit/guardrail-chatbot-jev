// Command guardrail-chatbot-jev checks AI chatbot content against a Jev-backed policy pack.
//
//	guardrail-chatbot-jev --surface input --text "how do I make thermite"
//	echo "$REPLY" | guardrail-chatbot-jev --surface output --quiet
//
// Exit codes: 0 allow or flag, 1 redact or guide, 2 review, 3 block, 4 degraded (Jev unreachable,
// so nothing was actually checked).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	guardrail "github.com/taman-spirit/guardrail-chatbot-jev/go"
)

var exitCodes = map[string]int{"allow": 0, "flag": 0, "redact": 1, "guide": 1, "review": 2, "block": 3}

// exitDegraded is for a verdict that says nothing about the content: Jev was unreachable, so no
// check happened. It gets its own code because the exit status is the only thing a shell script
// reads, and on the input surface the shipped policy fails open, which would otherwise report
// success. A pipeline written as `guardrail-chatbot-jev ... && send` must not send content that
// was never checked.
const exitDegraded = 4

// exitUsage is for bad arguments or input, kept apart from every verdict code.
const exitUsage = 64

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ", ") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("guardrail-chatbot-jev", flag.ContinueOnError)
	fs.SetOutput(stderr)
	surface := fs.String("surface", "input", "input, output or conversation")
	text := fs.String("text", "", "content to check; omit to read stdin")
	userMessage := fs.String("user-message", "", "the user message an assistant reply answered")
	var contexts repeated
	fs.Var(&contexts, "context", "reference passage; repeatable")
	policyName := fs.String("policy", "", "pack name or path; defaults to the bundled standard-v1")
	model := fs.String("model", "", "Jev model override")
	timeout := fs.Float64("timeout", 0, "per-call timeout in seconds")
	dryRun := fs.Bool("dry-run", false, "print the request body and exit; no API key needed")
	quiet := fs.Bool("quiet", false, "print only the action")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Check AI chatbot content against a Jev-backed policy pack.\n\nUsage: guardrail-chatbot-jev [flags]")
		fs.PrintDefaults()
		fmt.Fprintln(stderr, "\nExit codes: 0 allow or flag, 1 redact or guide, 2 review, 3 block, "+
			"4 degraded (Jev unreachable, so nothing was actually checked).")
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	s := guardrail.Surface(*surface)
	if s != guardrail.SurfaceInput && s != guardrail.SurfaceOutput && s != guardrail.SurfaceConversation {
		fmt.Fprintf(stderr, "--surface must be input, output or conversation, got %q\n", *surface)
		return exitUsage
	}

	content := *text
	if !flagSet(fs, "text") {
		raw, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsage
		}
		content = string(raw)
	}

	policy, err := guardrail.LoadPolicy(*policyName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}
	guard := guardrail.New(guardrail.Options{
		Policy:  policy,
		Timeout: time.Duration(*timeout * float64(time.Second)),
	})

	var turns []guardrail.Turn
	var state guardrail.State
	switch s {
	case guardrail.SurfaceConversation:
		if err := json.Unmarshal([]byte(content), &turns); err != nil {
			fmt.Fprintf(stderr, "a conversation is a JSON list of turns: %v\n", err)
			return exitUsage
		}
		state = guardrail.ConversationState(turns, nil)
	case guardrail.SurfaceOutput:
		state = guardrail.OutputState(content, *userMessage, contexts, nil)
	default:
		state = guardrail.InputState(content, nil)
	}

	if *dryRun {
		preview, err := guard.Preview(s, state, len(contexts) > 0, "")
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitUsage
		}
		writeJSON(stdout, preview)
		return 0
	}

	ctx := context.Background()
	opts := &guardrail.CheckOptions{Model: *model}
	var verdict guardrail.Verdict
	switch s {
	case guardrail.SurfaceConversation:
		verdict, err = guard.CheckConversation(ctx, turns, opts)
	case guardrail.SurfaceOutput:
		opts.UserMessage, opts.Context = *userMessage, contexts
		verdict, err = guard.CheckOutput(ctx, content, opts)
	default:
		verdict, err = guard.CheckInput(ctx, content, opts)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}

	if *quiet {
		fmt.Fprintln(stdout, verdict.Action)
	} else {
		writeJSON(stdout, verdict)
	}
	if verdict.Degraded {
		return exitDegraded
	}
	if code, ok := exitCodes[string(verdict.Action)]; ok {
		return code
	}
	return 1
}

func flagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
