package guardrail

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func record(id string, expected Action, parts A) Record {
	return Record{Surface: SurfaceInput, Answers: answers(parts), Expected: expected, ID: id}
}

func labelled() []Record {
	// A small set with one of each outcome, including a miss and an over-block.
	return []Record{
		record("safe-1", Allow, nil),
		record("safe-2", Allow, A{"hazard": hazard(P{"ncr": 0.02})}),
		record("over-1", Allow, A{"hazard": hazard(P{"hte": 0.62}), "actionability": score(2)}),
		record("miss-1", Block, A{"hazard": hazard(P{"ncr": 0.2})}),
		record("hit-1", Block, A{"hazard": hazard(P{"iwp": 0.6}), "s_iwp": noul(0.85), "actionability": score(2)}),
		record("flag-1", Flag, A{"hazard": hazard(P{"vcr": 0.16})}),
	}
}

func TestRecordingTransportKeepsTheExchange(t *testing.T) {
	recording := &RecordingTransport{Inner: NewRecordedTransport(clean)}
	_, _ = New(Options{Transport: recording}).CheckInput(ctx, "xin chào", nil)
	records := recording.Records()
	if len(records) != 1 || records[0].State["user_message"] != "xin chào" || records[0].Answers["hazard"] == nil {
		t.Fatalf("got %+v", records)
	}
}

func TestARecordingReplaysToTheSameVerdict(t *testing.T) {
	// The point of recording: the replay must agree with the live run it came from.
	p := bundled(t)
	raw := Answers{"hazard": {"type": "choice", "choice": "ncr", "confidence": 0.88, "probabilities": map[string]any{"ncr": 0.55, "none": 0.45}}}
	recording := &RecordingTransport{Inner: NewRecordedTransport(raw)}
	live, _ := New(Options{Policy: p, Transport: recording}).CheckInput(ctx, "...", nil)
	replayed := Replay(p, []Record{{Surface: SurfaceInput, Answers: recording.Records()[0].Answers}})[0]
	if replayed.Action != live.Action || replayed.Route != live.Route ||
		strings.Join(replayed.Categories(), ",") != strings.Join(live.Categories(), ",") {
		t.Fatalf("live=%+v replayed=%+v", live, replayed)
	}
}

func TestRecordsLoadFromJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.jsonl")
	_ = os.WriteFile(path, []byte(`{"id": "c1", "surface": "output", "expected_action": "review", "expected_category": "prv", "answers": {"hazard": {"type": "choice", "choice": "prv", "confidence": 0.9, "probabilities": {"prv": 0.4}}}}`+"\n\n"), 0o600)
	loaded, err := LoadRecords(path, "")
	if err != nil {
		t.Fatal(err)
	}
	r := loaded[0]
	if len(loaded) != 1 || r.ID != "c1" || r.Surface != SurfaceOutput || r.Expected != Review || r.ExpectedCategory != "prv" {
		t.Fatalf("got %+v", loaded)
	}
}

func TestScoreSeparatesTheTwoKindsOfError(t *testing.T) {
	set := labelled()
	report := Score(set, Replay(bundled(t), set))
	if report.Total != 6 || !contains(report.CriticalMisses, "miss-1") || !contains(report.OverBlocks, "over-1") ||
		report.Under < 1 || report.Over < 1 || report.ReviewRate() < 0 || report.ReviewRate() > 1 {
		t.Fatalf("got %+v", report)
	}
	total := 0
	for label, row := range report.Confusion {
		if !validAction(label) {
			t.Fatalf("confusion keyed by %q", label)
		}
		for _, n := range row {
			total += n
		}
	}
	if total != 6 {
		t.Fatalf("confusion holds %d cases", total)
	}
}

func TestUnlabelledRecordsAreCountedButNotScored(t *testing.T) {
	set := []Record{{Surface: SurfaceInput, Answers: answers(nil)}}
	report := Score(set, Replay(bundled(t), set))
	if report.Total != 1 || report.Exact != 0 || len(report.Confusion) != 0 {
		t.Fatalf("got %+v", report)
	}
}

func TestOverrideChangesTheOutcome(t *testing.T) {
	p := bundled(t)
	set := []Record{record("m", Block, A{"hazard": hazard(P{"ncr": 0.2}), "actionability": score(2)})}
	if Replay(p, set)[0].Action == Block {
		t.Fatal("already blocked before the override")
	}
	loosened, err := Override(p, map[string]float64{"ncr.input.block": 0.15})
	if err != nil {
		t.Fatal(err)
	}
	if Replay(loosened, set)[0].Action != Block {
		t.Fatal("override did not take")
	}
}

func TestOverrideKeepsTheBandsOrderedAndTheOriginalAlone(t *testing.T) {
	p := bundled(t)
	before, _ := p.Categories["ncr"].Threshold(SurfaceInput)
	tightened, err := Override(p, map[string]float64{"ncr.input.block": 0.1})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := tightened.Categories["ncr"].Threshold(SurfaceInput)
	if !(b.Block >= b.Review && b.Review >= b.Flag) {
		t.Fatalf("bands inverted: %+v", b)
	}
	if after, _ := p.Categories["ncr"].Threshold(SurfaceInput); after != before {
		t.Fatal("override changed the original")
	}
	// The override must not disturb the order of labels sent to Jev.
	questions, _ := BuildQuestions(tightened, SurfaceInput, false, "")
	if labels := criteriaLabels(t, questions["intent"]); labels[0] != "benign" {
		t.Fatalf("got %v", labels)
	}
}

func TestABadAxisIsRejected(t *testing.T) {
	p := bundled(t)
	if _, err := Override(p, map[string]float64{"ncr.input": 0.5}); err == nil || !strings.Contains(err.Error(), "category.surface.band") {
		t.Fatalf("got %v", err)
	}
	if _, err := Override(p, map[string]float64{"nope.input.block": 0.5}); err == nil {
		t.Fatal("unknown category accepted")
	}
}

func TestSweepIsMonotonicInTheRightDirection(t *testing.T) {
	// Raising a block threshold can only block less, never more.
	values, _ := Grid(0.1, 0.6, 0.1)
	rows, err := Sweep(bundled(t), labelled(), "ncr.input.block", values)
	if err != nil {
		t.Fatal(err)
	}
	var blocked []int
	for _, r := range rows {
		blocked = append(blocked, r.Report.Actions[Block])
	}
	if !sort.SliceIsSorted(blocked, func(i, j int) bool { return blocked[i] > blocked[j] }) {
		t.Fatalf("got %v", blocked)
	}
}

func categorised(id string, expected Action, category string, parts A) Record {
	r := record(id, expected, parts)
	r.ExpectedCategory = category
	return r
}

func TestSeparationSpotsAnOverlapAndACleanSplit(t *testing.T) {
	p := bundled(t)
	overlap := Separation(p, []Record{
		categorised("a", Block, "ncr", A{"hazard": hazard(P{"ncr": 0.3})}),
		categorised("b", Allow, "", A{"hazard": hazard(P{"ncr": 0.35})}),
	}, "ncr", SurfaceInput)
	if overlap.Basis != "category" || overlap.Separated || overlap.ShouldFire.N != 1 || overlap.ShouldNotFire.N != 1 {
		t.Fatalf("got %+v", overlap)
	}
	split := Separation(p, []Record{
		categorised("a", Block, "ncr", A{"hazard": hazard(P{"ncr": 0.8})}),
		categorised("b", Allow, "", A{"hazard": hazard(P{"ncr": 0.02})}),
	}, "ncr", SurfaceInput)
	if !split.Separated {
		t.Fatalf("got %+v", split)
	}
}

func TestSeparationCountsOnlyTheCasesACategoryOwns(t *testing.T) {
	// A self-harm case must not be held against the fraud category.
	p := bundled(t)
	set := []Record{
		categorised("fraud", Block, "ncr", A{"hazard": hazard(P{"ncr": 0.6})}),
		categorised("harm", Block, "ssh", A{"hazard": hazard(P{"ssh": 0.6})}),
		categorised("safe", Allow, "", nil),
	}
	byCategory := Separation(p, set, "ncr", SurfaceInput)
	if byCategory.ShouldFire.N != 1 || !byCategory.Separated {
		t.Fatalf("got %+v", byCategory)
	}
	for i := range set {
		set[i].ExpectedCategory = ""
	}
	byAction := Separation(p, set, "ncr", SurfaceInput)
	if byAction.Basis != "action" || byAction.ShouldFire.N != 2 || byAction.Separated {
		t.Fatalf("got %+v", byAction)
	}
}

func TestTheLabelledSetsUseKnownCategories(t *testing.T) {
	// A typo in expected_category would silently make a category look like it never fires.
	p := bundled(t)
	paths, _ := filepath.Glob("../examples/*.jsonl")
	for _, path := range paths {
		records, err := LoadRecords(path, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range records {
			if r.ExpectedCategory != "" {
				if _, ok := p.Categories[r.ExpectedCategory]; !ok {
					t.Fatalf("%s: %s -> %q", path, r.ID, r.ExpectedCategory)
				}
			}
		}
	}
}

func TestProbabilityTakesTheHigherOfChoiceAndSentinel(t *testing.T) {
	r := Record{Surface: SurfaceInput, Answers: answers(A{"hazard": hazard(P{"cse": 0.1}), "s_cse": noul(0.7)})}
	if !approx(ProbabilityOf(r, "cse"), 0.7) || ProbabilityOf(r, "ncr") != 0 {
		t.Fatal("wrong probability")
	}
}

func TestGridIsInclusive(t *testing.T) {
	values, _ := Grid(0.1, 0.3, 0.1)
	if len(values) != 3 || values[0] != 0.1 || values[2] != 0.3 {
		t.Fatalf("got %v", values)
	}
	if _, err := Grid(0.1, 0.3, 0); err == nil {
		t.Fatal("zero step accepted")
	}
}
