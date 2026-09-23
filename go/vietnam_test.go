package guardrail

// The Viet Nam compliance pack and its prewritten replies, against recorded Jev answers.
//
// Most of these are about what must not be blocked. A compliance pack that stops ordinary
// questions about Trường Sa's weather, a leader's title or a bank's hotline fails its users as
// surely as one that lets a violation through.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func vn(t *testing.T) *Policy {
	t.Helper()
	p, err := BundledPolicy("vietnam-compliance-v1")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func vnResponder(t *testing.T, crisis string) *Responder {
	t.Helper()
	r, err := NewResponder(vn(t), crisis)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// vnAnswers adds neutral values for the pack's own signals to the shared defaults.
func vnAnswers(parts A) Answers {
	base := A{
		"neutral_mention":      noul(0.1),
		"sovereignty_question": noul(0.02),
		"data_subject":         choice("none"),
	}
	for k, v := range parts {
		base[k] = v
	}
	return answers(base)
}

func vs(v ...Verdict) []Verdict { return v }

// -- the pack --------------------------------------------------------------------

func TestTheVietnamPackKeepsTheShippedTaxonomy(t *testing.T) {
	p := vn(t)
	for id := range bundled(t).Categories {
		if _, ok := p.Categories[id]; !ok {
			t.Fatalf("dropped %s", id)
		}
	}
	for _, id := range []string{"vsv", "vas", "vld", "vcs", "vai"} {
		if _, ok := p.Categories[id]; !ok {
			t.Fatalf("missing %s", id)
		}
	}
	for _, s := range Surfaces {
		var ids []string
		for _, c := range p.Sentinels(s) {
			ids = append(ids, c.ID)
		}
		if !contains(ids, "vsv") || !contains(ids, "vld") {
			t.Fatalf("%s sentinels: %v", s, ids)
		}
	}
}

func TestTheVietnamPackMatchesTheCanonicalCopy(t *testing.T) {
	canonical, err := os.ReadFile("../policies/vietnam-compliance-v1.json")
	if err != nil {
		t.Skip("not running inside the repository")
	}
	shipped, _ := os.ReadFile(filepath.Join("policies", "vietnam-compliance-v1.json"))
	if string(canonical) != string(shipped) {
		t.Fatal("go/policies/vietnam-compliance-v1.json drifted; run scripts/sync-policies.sh")
	}
}

func TestAnIncompleteResponsesSectionIsRefused(t *testing.T) {
	var pack map[string]any
	data, _ := bundledPacks.ReadFile("policies/vietnam-compliance-v1.json")
	_ = json.Unmarshal(data, &pack)
	leaders := pack["responses"].(map[string]any)["groups"].(map[string]any)["leaders"].(map[string]any)
	delete(leaders["text"].(map[string]any), "zh")
	p, err := NewPolicy(pack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewResponder(p, ""); err == nil || !strings.Contains(err.Error(), "leaders.zh") {
		t.Fatalf("got %v", err)
	}
	if _, err := NewResponder(bundled(t), ""); err == nil {
		t.Fatal("a pack without responses was accepted")
	}
}

// -- the sovereignty affirmation --------------------------------------------------------

func TestTheAffirmationCarriesEveryReference(t *testing.T) {
	r := vnResponder(t, "")
	needles := map[string][]string{
		"vi": {"Hiến pháp năm 2013", "18/2012/QH13", "UNCLOS", "12/7/2016", "06/2003/QH11", "Hoàng Sa", "Trường Sa", "đặc khu"},
		"en": {"2013 Constitution", "18/2012/QH13", "UNCLOS", "12 July 2016", "06/2003/QH11", "Hoàng Sa", "Trường Sa", "special zone"},
		"zh": {"2013年《宪法》", "18/2012/QH13", "UNCLOS", "2016年7月12日", "06/2003/QH11", "黄沙群岛", "长沙群岛", "特区"},
	}
	for lang, want := range needles {
		text := r.Affirmation(lang)
		for _, n := range want {
			if !strings.Contains(text, n) {
				t.Fatalf("%s: %q missing", lang, n)
			}
		}
	}
	for _, foreign := range []string{"西沙", "南沙", "三沙"} {
		if strings.Contains(r.Affirmation("zh"), foreign) {
			t.Fatalf("zh uses %s", foreign)
		}
	}
}

// -- sovereignty paths --------------------------------------------------------------------

func TestWeatherOnTruongSaIsNotTouched(t *testing.T) {
	// The motivating false positive: a place name is not a claim.
	v := decideOn(vn(t), SurfaceInput, vnAnswers(A{"neutral_mention": noul(0.97), "s_vsv": noul(0.02)}))
	if v.Action != Allow {
		t.Fatalf("got %s", v.Action)
	}
	reply := "Trường Sa hôm nay nắng, gió cấp 4."
	if got := vnResponder(t, "").Compose(reply, vs(v), "vi"); got != reply {
		t.Fatalf("no affirmation bolted onto a weather answer: %q", got)
	}
}

func TestANeutralMentionIsNeverBlocked(t *testing.T) {
	v := decideOn(vn(t), SurfaceInput, vnAnswers(A{"neutral_mention": noul(0.95), "s_vsv": noul(0.6)}))
	if v.Action != Flag || !v.Deliverable() || !v.HasRule("neutral-mention-is-not-a-violation") {
		t.Fatalf("got %+v", v)
	}
}

func TestAQuestionAboutOwnershipGetsTheFixedAffirmation(t *testing.T) {
	r := vnResponder(t, "")
	v := decideOn(vn(t), SurfaceInput, vnAnswers(A{"sovereignty_question": noul(0.93)}))
	if v.Action != Allow {
		t.Fatalf("asking is not a violation: %s", v.Action)
	}
	got := r.Compose("Câu trả lời của mô hình.", vs(v), "vi")
	if !strings.HasPrefix(got, "Câu trả lời của mô hình.") || !strings.HasSuffix(got, r.Affirmation("vi")) {
		t.Fatalf("got %q", got)
	}
}

func TestAReplyCedingTerritoryIsReplacedAndAffirmed(t *testing.T) {
	r := vnResponder(t, "")
	v := decideOn(vn(t), SurfaceOutput, vnAnswers(A{"s_vsv": noul(0.9), "refusal": noul(0)}))
	if v.Action != Block || v.Deliverable() {
		t.Fatalf("got %+v", v)
	}
	got := r.Compose("Hoàng Sa thuộc về nước khác.", vs(v), "vi")
	if strings.Contains(got, "thuộc về nước khác") || !strings.HasPrefix(got, "Mình không thể đưa ra nội dung này") ||
		!strings.HasSuffix(got, r.Affirmation("vi")) {
		t.Fatalf("got %q", got)
	}
}

func TestNothingSoftensASovereigntyClaim(t *testing.T) {
	p := vn(t)
	if v := decideOn(p, SurfaceOutput, vnAnswers(A{"s_vsv": noul(0.9), "refusal": noul(0.95)})); v.Action != Block {
		t.Fatalf("a refusal excused it: %s", v.Action)
	}
	if v := decideOn(p, SurfaceInput, vnAnswers(A{"hazard": hazard(P{"vsv": 0.55}), "actionability": score(0)})); v.Action != Block {
		t.Fatalf("low actionability softened it: %s", v.Action)
	}
}

func TestASovereigntyReviewStillEndsWithTheAffirmation(t *testing.T) {
	r := vnResponder(t, "")
	v := decideOn(vn(t), SurfaceOutput, vnAnswers(A{"s_vsv": noul(0.3), "refusal": noul(0)}))
	text, ok := r.BlockingResponse(vs(v), "en")
	if v.Action != Review || !ok || !strings.HasPrefix(text, "Your request needs a further look") || !strings.HasSuffix(text, r.Affirmation("en")) {
		t.Fatalf("%s %q", v.Action, text)
	}
	conv := decideOn(vn(t), SurfaceConversation, vnAnswers(A{"s_vsv": noul(0.8)}))
	if text, ok := r.BlockingResponse(vs(conv), "zh"); !ok || !strings.HasSuffix(text, r.Affirmation("zh")) {
		t.Fatalf("conversation: %q", text)
	}
}

// -- the other groups -----------------------------------------------------------------------

func TestEachGroupHasItsOwnReply(t *testing.T) {
	r := vnResponder(t, "")
	p := vn(t)
	cases := map[string]string{
		"vas": "tuyên truyền chống Nhà nước",
		"vcs": "thông tin sai sự thật",
		"vai": "Luật Trí tuệ nhân tạo",
	}
	for category, phrase := range cases {
		v := decideOn(p, SurfaceInput, vnAnswers(A{"hazard": hazard(P{category: 0.8}), "actionability": score(2)}))
		text, ok := r.BlockingResponse(vs(v), "vi")
		if v.Action != Block || !ok || !strings.Contains(text, phrase) {
			t.Fatalf("%s: %s %q", category, v.Action, text)
		}
	}
	leader := decideOn(p, SurfaceInput, vnAnswers(A{"s_vld": noul(0.85)}))
	text, _ := r.BlockingResponse(vs(leader), "vi")
	if !strings.Contains(text, "xúc phạm lãnh tụ") || strings.Contains(text, "Hiến pháp") {
		t.Fatalf("leaders: %q", text)
	}
	if bio := decideOn(p, SurfaceInput, vnAnswers(A{"neutral_mention": noul(0.9), "s_vld": noul(0.03)})); bio.Action != Allow {
		t.Fatalf("a biography was not allowed: %s", bio.Action)
	}
}

// -- personal data: individuals only ------------------------------------------------------------

func TestACustomerServiceHotlineIsNotPersonalData(t *testing.T) {
	v := decideOn(vn(t), SurfaceOutput, vnAnswers(A{"s_prv": noul(0.45), "data_subject": choice("organization"), "refusal": noul(0)}))
	if v.Action != Allow || v.Route != RouteDeliver {
		t.Fatalf("a hotline must be neither blocked nor redacted: %+v", v)
	}
	reply := "Tổng đài chăm sóc khách hàng: 1800 1090."
	if got := vnResponder(t, "").Compose(reply, vs(v), "vi"); got != reply {
		t.Fatalf("got %q", got)
	}
}

func TestAnIndividualsDataIsStillProtected(t *testing.T) {
	p := vn(t)
	v := decideOn(p, SurfaceInput, vnAnswers(A{"s_prv": noul(0.85), "data_subject": choice("individual")}))
	text, ok := vnResponder(t, "").BlockingResponse(vs(v), "en")
	if v.Action != Block || !ok || !strings.Contains(text, "personal information about a specific person") {
		t.Fatalf("%s %q", v.Action, text)
	}
	// Saying the subject is a company must not also wave through a threat against it.
	threat := decideOn(p, SurfaceInput, vnAnswers(A{"hazard": hazard(P{"vcs": 0.8}), "data_subject": choice("organization"), "actionability": score(2)}))
	if threat.Action != Block {
		t.Fatalf("got %s", threat.Action)
	}
}

// -- self-harm, outages, language ---------------------------------------------------------------

func TestSelfHarmIsMetWithEmpathyAnd115(t *testing.T) {
	v := decideOn(vn(t), SurfaceInput, vnAnswers(A{"s_ssh": noul(0.8)}))
	r := vnResponder(t, "")
	text, ok := r.BlockingResponse(vs(v), "vi")
	if v.Route != RouteCrisisSupport || !ok || r.CrisisLine() != "115" {
		t.Fatalf("%s %v %s", v.Route, ok, r.CrisisLine())
	}
	if !strings.Contains(text, "hãy gọi 115") || strings.Contains(text, "Luật") || strings.Contains(text, "{crisis_line}") {
		t.Fatalf("got %q", text)
	}
	for _, lang := range []string{"vi", "en", "zh"} {
		custom, _ := vnResponder(t, "[verified line]").BlockingResponse(vs(v), lang)
		if !strings.Contains(custom, "[verified line]") || strings.Contains(custom, "115") {
			t.Fatalf("%s: one number to call, not two: %q", lang, custom)
		}
	}
}

func TestAnOutageSaysSoInsteadOfAccusingTheUser(t *testing.T) {
	v := ErrorVerdict(vn(t), SurfaceOutput, &Error{Msg: "down"}, 0)
	if text, _ := vnResponder(t, "").BlockingResponse(vs(v), "vi"); !strings.Contains(text, "tạm thời gián đoạn") {
		t.Fatalf("got %q", text)
	}
}

func TestDetectLanguage(t *testing.T) {
	cases := map[string]string{
		"Trường Sa thời tiết ra sao?":       "vi",
		"What is the weather on Truong Sa?": "en",
		"长沙群岛天气怎么样？":                        "zh",
		"":                                  "vi",
		"Hello, cho mình hỏi":               "vi",
	}
	for text, want := range cases {
		if got := DetectLanguage(text); got != want {
			t.Fatalf("%q: got %s, want %s", text, got, want)
		}
	}
	v := decideOn(vn(t), SurfaceInput, vnAnswers(A{"s_vld": noul(0.85)}))
	r := vnResponder(t, "")
	fr, _ := r.BlockingResponse(vs(v), "fr")
	vi, _ := r.BlockingResponse(vs(v), "vi")
	if fr != vi {
		t.Fatal("an unknown language must fall back to Vietnamese")
	}
}

// -- end to end ----------------------------------------------------------------------------------

func TestOneVietnamTurnThroughTheGuard(t *testing.T) {
	p := vn(t)
	transport := NewRecordedTransport(vnAnswers(A{"sovereignty_question": noul(0.9)}))
	r := vnResponder(t, "")
	question := "Quần đảo Trường Sa thuộc nước nào?"

	in, _ := New(Options{Policy: p, Transport: transport}).CheckInput(ctx, question, nil)
	if _, held := r.BlockingResponse(vs(in), DetectLanguage(question)); held {
		t.Fatal("the question itself goes to the model")
	}
	out, _ := New(Options{Policy: p, Transport: NewRecordedTransport(vnAnswers(A{"refusal": noul(0)}))}).
		CheckOutput(ctx, "Trường Sa thuộc Việt Nam.", &CheckOptions{UserMessage: question})
	sent := r.Compose("Trường Sa thuộc Việt Nam.", vs(in, out), DetectLanguage(question))
	if !strings.HasPrefix(sent, "Trường Sa thuộc Việt Nam.") || !strings.HasSuffix(sent, r.Affirmation("vi")) {
		t.Fatalf("got %q", sent)
	}
	for _, q := range []string{"s_vsv", "s_vld", "neutral_mention", "sovereignty_question", "data_subject"} {
		if _, ok := transport.Calls()[0].Questions[q]; !ok {
			t.Fatalf("%s not asked", q)
		}
	}
}

func TestTheVietnamLabelledSetIsWellFormed(t *testing.T) {
	records, err := LoadRecords("../examples/cases-vietnam.jsonl", "")
	if err != nil {
		t.Skip("not running inside the repository")
	}
	allowed := 0
	for _, r := range records {
		if !validAction(r.Expected) {
			t.Fatalf("%s: %q", r.ID, r.Expected)
		}
		if r.Expected == Allow {
			allowed++
		}
		if r.ExpectedCategory != "" {
			if _, ok := vn(t).Categories[r.ExpectedCategory]; !ok {
				t.Fatalf("%s: %q", r.ID, r.ExpectedCategory)
			}
		}
	}
	if len(records) < 30 || allowed*2 < len(records) {
		t.Fatalf("%d cases, %d allowed", len(records), allowed)
	}
}
