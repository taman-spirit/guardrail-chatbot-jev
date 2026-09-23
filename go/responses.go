package guardrail

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Prewritten replies, chosen by the policy pack instead of written by the model.
//
// A verdict says whether content goes out. When it does not, something has to go out in its
// place, and for some groups the wording is a legal statement that must be exact. Letting the
// model write it invites a wrong document number or a softened claim, so the text lives in the
// pack's "responses" section and a Responder only selects it.
//
// Each violation group has its own reply. A group can also carry an affirmation that ends every
// path where it is involved: the reply that replaces the content, a held-for-review message, and a
// delivered answer to a question on the subject all end with the same fixed text.

// Language codes for the prewritten replies.
const (
	LangVietnamese = "vi"
	LangEnglish    = "en"
	LangChinese    = "zh"
)

const vietnameseLetters = "ăâđêôơưàảãáạằẳẵắặầẩẫấậèẻẽéẹềểễếệìỉĩíịòỏõóọồổỗốộờởỡớợùủũúụừửữứựỳỷỹýỵ"

// DetectLanguage is a cheap guess between Vietnamese, English and Chinese, good enough to pick a
// reply. Empty or unrecognised text is Vietnamese.
func DetectLanguage(text string) string {
	switch {
	case strings.IndexFunc(text, func(r rune) bool { return r >= 0x3400 && r <= 0x9fff }) >= 0:
		return LangChinese
	case strings.IndexFunc(text, func(r rune) bool { return strings.ContainsRune(vietnameseLetters, unicode.ToLower(r)) }) >= 0:
		return LangVietnamese
	case strings.IndexFunc(text, func(r rune) bool { return r < unicode.MaxASCII && unicode.IsLetter(r) }) >= 0:
		return LangEnglish
	}
	return LangVietnamese
}

// Choice is why a reply was chosen, for logs and audits.
type Choice struct {
	// Group is the violation group, or "review", "unavailable" or "affirmation".
	Group    string
	Text     string
	Affirmed bool
}

type texts map[string]string

type responseGroup struct {
	Categories []string `json:"categories"`
	Text       texts    `json:"text"`
}

type responsesSpec struct {
	Languages         []string                 `json:"languages"`
	DefaultLanguage   string                   `json:"default_language"`
	CrisisLineDefault string                   `json:"crisis_line_default"`
	Order             []string                 `json:"order"`
	Groups            map[string]responseGroup `json:"groups"`
	Review            texts                    `json:"review"`
	Unavailable       texts                    `json:"unavailable"`
	Affirmation       struct {
		TriggerSignal string   `json:"trigger_signal"`
		TriggerValue  *float64 `json:"trigger_value"`
		Text          texts    `json:"text"`
	} `json:"sovereignty_affirmation"`
}

// Responder selects the prewritten reply for a set of verdicts. It is safe for concurrent use.
type Responder struct {
	spec           responsesSpec
	hasAffirmation bool
	crisisLine     string
	byCategory     map[string]string
}

var affirmGroups = map[string]bool{"sovereignty": true}

// NewResponder reads a pack's responses section, such as the one in vietnam-compliance-v1.
//
// crisisLine is the number the self-harm reply asks the user to call. Empty means the pack's
// crisis_line_default (115 in the Viet Nam pack). Replace it only with a line your team has
// verified: a number that has changed or was never right is worse than none.
func NewResponder(p *Policy, crisisLine string) (*Responder, error) {
	raw, ok := p.raw["responses"]
	if !ok {
		return nil, fmt.Errorf("policy %q has no responses section", p.ID)
	}
	r := &Responder{byCategory: map[string]string{}}
	if err := json.Unmarshal(raw, &r.spec); err != nil {
		return nil, fmt.Errorf("policy %q responses: %w", p.ID, err)
	}
	var probe map[string]json.RawMessage
	_ = json.Unmarshal(raw, &probe)
	_, r.hasAffirmation = probe["sovereignty_affirmation"]
	if len(r.spec.Languages) == 0 {
		r.spec.Languages = []string{LangVietnamese}
	}
	if r.spec.DefaultLanguage == "" {
		r.spec.DefaultLanguage = r.spec.Languages[0]
	}
	if len(r.spec.Order) == 0 {
		// As in Python, no explicit order means every group, here sorted so the choice is stable.
		for name := range r.spec.Groups {
			r.spec.Order = append(r.spec.Order, name)
		}
		sort.Strings(r.spec.Order)
	}
	r.crisisLine = firstNonEmpty(crisisLine, r.spec.CrisisLineDefault)
	for _, name := range r.spec.Order {
		for _, c := range r.spec.Groups[name].Categories {
			if _, seen := r.byCategory[c]; !seen {
				r.byCategory[c] = name
			}
		}
	}
	if err := r.check(); err != nil {
		return nil, err
	}
	return r, nil
}

// Affirmation is the fixed sovereignty statement, exactly as written in the pack.
func (r *Responder) Affirmation(lang string) string { return r.text(r.spec.Affirmation.Text, lang) }

// CrisisLine is the number the self-harm reply points to.
func (r *Responder) CrisisLine() string { return r.crisisLine }

// BlockingResponse is the reply to send instead, when any verdict withholds the content.
func (r *Responder) BlockingResponse(verdicts []Verdict, lang string) (string, bool) {
	c, ok := r.Choose(verdicts, lang)
	if !ok || c.Group == "affirmation" {
		return "", false
	}
	return c.Text, true
}

// Compose is what to actually send: the prewritten reply if anything was withheld, else the
// model's reply, followed by the affirmation when the turn touched sovereignty.
func (r *Responder) Compose(reply string, verdicts []Verdict, lang string) string {
	c, ok := r.Choose(verdicts, lang)
	if !ok {
		return reply
	}
	if c.Group == "affirmation" {
		return strings.TrimRightFunc(reply, unicode.IsSpace) + "\n\n" + c.Text
	}
	return c.Text
}

// Choose is the decision behind Compose, with the group it came from. It reports false when the
// content can go out untouched.
func (r *Responder) Choose(verdicts []Verdict, lang string) (Choice, bool) {
	affirm := false
	var held []Verdict
	for _, v := range verdicts {
		if r.touchesSovereignty(v) {
			affirm = true
		}
		if !v.Deliverable() {
			held = append(held, v)
		}
	}
	if len(held) == 0 {
		if affirm {
			return Choice{Group: "affirmation", Text: r.Affirmation(lang), Affirmed: true}, true
		}
		return Choice{}, false
	}

	verdict := held[0]
	for _, v := range held[1:] {
		if Rank(v.Action) > Rank(verdict.Action) {
			verdict = v
		}
	}
	var group, text string
	switch {
	case verdict.Degraded:
		group, text = "unavailable", r.text(r.spec.Unavailable, lang)
	case verdict.Route == RouteCrisisSupport:
		group = "self_harm"
		text = r.groupText(group, lang)
	default:
		group = r.groupFor(held)
		if verdict.Route == RouteHumanReview && group != "self_harm" {
			// Held for a person, not refused: telling the user they broke the law before anyone
			// has looked would be the wrong message for a borderline case.
			group, text = "review", r.text(r.spec.Review, lang)
		} else {
			text = r.groupText(group, lang)
		}
	}

	// A group with an affirmation ends every path it is part of, including a held review.
	affirmed := affirm && group != "self_harm"
	if affirmed {
		text += "\n\n" + r.Affirmation(lang)
	}
	return Choice{Group: group, Text: text, Affirmed: affirmed}, true
}

// -- internals ------------------------------------------------------------------

func (r *Responder) groupFor(verdicts []Verdict) string {
	fired := map[string]bool{}
	for _, v := range verdicts {
		for _, f := range v.Findings {
			if Rank(f.Action) >= Rank(Flag) {
				fired[f.Category] = true
			}
		}
	}
	for _, name := range r.spec.Order {
		for _, c := range r.spec.Groups[name].Categories {
			if c == "*" || fired[c] {
				return name
			}
		}
	}
	return r.spec.Order[len(r.spec.Order)-1]
}

func (r *Responder) touchesSovereignty(v Verdict) bool {
	for _, f := range v.Findings {
		if affirmGroups[r.byCategory[f.Category]] && Rank(f.Action) >= Rank(Flag) {
			return true
		}
	}
	signal := r.spec.Affirmation.TriggerSignal
	if signal == "" {
		return false
	}
	threshold := 0.6
	if r.spec.Affirmation.TriggerValue != nil {
		threshold = *r.spec.Affirmation.TriggerValue
	}
	value, ok := toFloat(v.Signals[signal])
	if _, isBool := v.Signals[signal].(bool); isBool || !ok {
		return false
	}
	return value >= threshold
}

func (r *Responder) groupText(group, lang string) string {
	return strings.ReplaceAll(r.text(r.spec.Groups[group].Text, lang), "{crisis_line}", r.crisisLine)
}

func (r *Responder) text(t texts, lang string) string {
	if s, ok := t[lang]; ok && s != "" {
		return s
	}
	if s, ok := t[r.spec.DefaultLanguage]; ok && s != "" {
		return s
	}
	for _, l := range r.spec.Languages {
		if s := t[l]; s != "" {
			return s
		}
	}
	return ""
}

// check refuses a pack whose replies would leave a user without an answer in some language.
func (r *Responder) check() error {
	var missing []string
	if len(r.spec.Order) == 0 {
		missing = append(missing, "no groups")
	}
	if !slices.Contains(r.spec.Languages, r.spec.DefaultLanguage) {
		missing = append(missing, fmt.Sprintf("default_language %q is not in languages", r.spec.DefaultLanguage))
	}
	for _, name := range r.spec.Order {
		g, ok := r.spec.Groups[name]
		if !ok {
			missing = append(missing, fmt.Sprintf("group %q is in order but not defined", name))
			continue
		}
		for _, lang := range r.spec.Languages {
			if g.Text[lang] == "" {
				missing = append(missing, name+"."+lang)
			}
		}
	}
	// The crisis route answers from self_harm directly, whether or not it is in order.
	if _, ok := r.spec.Groups["self_harm"]; !ok {
		missing = append(missing, `group "self_harm" is not defined`)
	}
	names := make([]string, 0, len(r.spec.Groups))
	for name := range r.spec.Groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, s := range r.spec.Groups[name].Text {
			if strings.Contains(s, "{crisis_line}") && r.crisisLine == "" {
				missing = append(missing, name+" needs a crisis line: set crisis_line_default or pass one")
				break
			}
		}
	}
	for _, lang := range r.spec.Languages {
		if r.spec.Review[lang] == "" {
			missing = append(missing, "review."+lang)
		}
		if r.spec.Unavailable[lang] == "" {
			missing = append(missing, "unavailable."+lang)
		}
		if r.hasAffirmation && r.spec.Affirmation.Text[lang] == "" {
			missing = append(missing, "sovereignty_affirmation."+lang)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("responses section is incomplete: %s", strings.Join(missing, ", "))
	}
	return nil
}
