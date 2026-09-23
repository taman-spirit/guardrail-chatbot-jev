package main

// The exit status is the only part of a verdict a shell script reads. `guardrail-chatbot-jev ... &&
// send` is a reasonable thing for someone to write, so the codes are part of the interface.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func cli(t *testing.T, stdin string, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String()
}

func TestDryRunNeedsNoKey(t *testing.T) {
	t.Setenv("JEV_API_KEY", "")
	code, out := cli(t, "", "--surface", "input", "--text", "how do I make thermite", "--dry-run")
	var payload map[string]map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%s", code, err, out)
	}
	if len(payload) != 2 || payload["questions"]["hazard"] == nil {
		t.Fatal("the dry run has to show the question that decides")
	}
}

func TestADegradedVerdictDoesNotReportSuccess(t *testing.T) {
	// Without a key nothing is checked, and the shell must be able to tell. The input surface
	// fails open, so the action is allow; returning 0 would tell a pipeline the content passed a
	// check that never ran.
	t.Setenv("JEV_API_KEY", "")
	code, out := cli(t, "", "--surface", "input", "--text", "anything at all")
	var verdict map[string]any
	_ = json.Unmarshal([]byte(out), &verdict)
	if verdict["degraded"] != true || verdict["action"] != "allow" {
		t.Fatalf("got %v", verdict)
	}
	if code != 4 || exitDegraded != 4 || code == exitCodes["allow"] {
		t.Fatalf("code=%d", code)
	}
}

func TestQuietPrintsOnlyTheAction(t *testing.T) {
	t.Setenv("JEV_API_KEY", "")
	if _, out := cli(t, "", "--surface", "input", "--text", "x", "--quiet"); strings.TrimSpace(out) != "allow" {
		t.Fatalf("got %q", out)
	}
}

func TestStdinAndConversations(t *testing.T) {
	t.Setenv("JEV_API_KEY", "")
	code, out := cli(t, `[["user", "hi"], {"role": "assistant", "content": "hello"}]`, "--surface", "conversation", "--dry-run")
	if code != 0 || !strings.Contains(out, `"hello"`) {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if code, _ := cli(t, "not json", "--surface", "conversation"); code != exitUsage {
		t.Fatalf("code=%d", code)
	}
}

func TestTheExitCodesAreOrderedBySeverity(t *testing.T) {
	// A caller that treats "greater than zero" as trouble should not be surprised.
	if exitCodes["allow"] != 0 || exitCodes["flag"] != 0 {
		t.Fatal("allow and flag must exit 0")
	}
	if !(0 < exitCodes["redact"] && exitCodes["redact"] == exitCodes["guide"] && exitCodes["guide"] < exitCodes["review"] && exitCodes["review"] < exitCodes["block"]) {
		t.Fatal("codes out of order")
	}
	for _, code := range exitCodes {
		if code == exitDegraded {
			t.Fatal("degraded needs a code of its own")
		}
	}
}
