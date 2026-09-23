package guardrail

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed policies/*.json
var bundledPacks embed.FS

const defaultPack = "standard-v1"

// Bands are the probabilities at which a category resolves to each action.
type Bands struct {
	Block  float64 `json:"block"`
	Review float64 `json:"review"`
	Flag   float64 `json:"flag"`
}

// Category is one hazard in the taxonomy.
type Category struct {
	ID                   string
	Name                 string
	Description          string
	Refs                 []string
	Surfaces             map[Surface]bool
	Weight               float64
	BaseSeverity         int
	Sentinel             bool
	SentinelInstructions string
	Thresholds           map[string]Bands
	Route                Route
	NeverBelow           Action
	Enabled              bool
}

// Threshold returns the bands for a surface, falling back to "default".
func (c *Category) Threshold(surface Surface) (Bands, bool) {
	if b, ok := c.Thresholds[string(surface)]; ok {
		return b, true
	}
	b, ok := c.Thresholds["default"]
	return b, ok
}

// Applies reports whether the category is enabled and scored on a surface.
func (c *Category) Applies(surface Surface) bool {
	return c.Enabled && c.Surfaces[surface]
}

// Signal is a question asked alongside the hazard choice, read by the policy's rules.
type Signal struct {
	Name            string
	Type            string
	Surfaces        []string
	Instructions    string
	RequiresContext bool
	CriteriaRef     string
	// Criteria is kept as raw JSON so that label order survives into the request.
	Criteria    json.RawMessage
	hasCriteria bool
}

// Rule adjusts a verdict when a signal crosses a value.
type Rule struct {
	ID               string
	Signal           string
	Op               string
	Value            any
	ExceptCategories map[string]bool
	Upgrade          int
	Downgrade        int
	CapAction        Action
	FloorAction      Action
	AddFinding       string
}

// Policy is a parsed policy pack.
//
// A policy pack is a JSON document: the hazard taxonomy, the thresholds that turn a probability
// into an action, and the rules that adjust it. The descriptions in the pack are also the text
// sent to Jev as question criteria, so editing the pack changes both what the model is asked and
// how the answer is judged.
type Policy struct {
	ID          string
	Version     string
	Name        string
	ContentNote string
	Defaults    map[string]any
	Signals     map[string]*Signal
	Rules       []Rule
	Routes      map[string]string
	Categories  map[string]*Category
	// Scales are the pack's "*_scale" entries, kept raw so label order survives.
	Scales map[string]json.RawMessage

	raw map[string]json.RawMessage
}

// ParsePolicy reads a policy pack from JSON.
func ParsePolicy(data []byte) (*Policy, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("policy pack is not a JSON object: %w", err)
	}
	return fromRaw(raw)
}

// NewPolicy builds a policy from a decoded pack, such as one assembled in code.
func NewPolicy(data map[string]any) (*Policy, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return ParsePolicy(encoded)
}

// LoadPolicy loads the bundled pack (""), a bundled pack by name, or a path to a JSON file.
func LoadPolicy(source string) (*Policy, error) {
	if source == "" {
		return BundledPolicy(defaultPack)
	}
	ext := strings.ToLower(filepath.Ext(source))
	if !strings.ContainsAny(source, `/\`) && ext != ".json" && ext != ".yaml" && ext != ".yml" {
		return BundledPolicy(source)
	}
	if ext == ".yaml" || ext == ".yml" {
		// Staying dependency-free matters more here than reading YAML directly.
		return nil, fmt.Errorf("YAML policy packs are not supported by the Go package; convert %s to JSON", source)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	return ParsePolicy(data)
}

// BundledPolicy loads a pack shipped inside the package.
func BundledPolicy(name string) (*Policy, error) {
	data, err := bundledPacks.ReadFile("policies/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("no bundled policy pack named %q", name)
	}
	return ParsePolicy(data)
}

// MustBundledPolicy is BundledPolicy for the standard pack, which is embedded and always valid.
func MustBundledPolicy() *Policy {
	p, err := BundledPolicy(defaultPack)
	if err != nil {
		panic(err)
	}
	return p
}

// QualifiedID is "id@version", the form carried on every verdict and in every cache key.
func (p *Policy) QualifiedID() string { return p.ID + "@" + p.Version }

// ForSurface lists enabled categories that apply to a surface, most serious first.
func (p *Policy) ForSurface(surface Surface) []*Category {
	var cats []*Category
	for _, c := range p.Categories {
		if c.Applies(surface) {
			cats = append(cats, c)
		}
	}
	sort.Slice(cats, func(i, j int) bool {
		a, b := cats[i], cats[j]
		if a.Weight != b.Weight {
			return a.Weight > b.Weight
		}
		if a.BaseSeverity != b.BaseSeverity {
			return a.BaseSeverity > b.BaseSeverity
		}
		return a.ID < b.ID
	})
	return cats
}

// Sentinels lists the categories that get a dedicated yes/no question on this surface.
//
// A choice question picks one label. Content can carry more than one hazard at once, and the
// ones where a miss is unacceptable get their own independent question.
func (p *Policy) Sentinels(surface Surface) []*Category {
	var out []*Category
	for _, c := range p.ForSurface(surface) {
		if c.Sentinel && c.SentinelInstructions != "" {
			out = append(out, c)
		}
	}
	return out
}

// SignalsFor lists the signal definitions that apply to a surface.
func (p *Policy) SignalsFor(surface Surface, hasContext bool) map[string]*Signal {
	out := map[string]*Signal{}
	for name, spec := range p.Signals {
		applies := false
		for _, s := range spec.Surfaces {
			if s == string(surface) {
				applies = true
				break
			}
		}
		if !applies || (spec.RequiresContext && !hasContext) {
			continue
		}
		out[name] = spec
	}
	return out
}

// CriteriaFor resolves a signal's criteria, following criteria_ref into the pack's scales.
func (p *Policy) CriteriaFor(spec *Signal) json.RawMessage {
	if spec.hasCriteria {
		return spec.Criteria
	}
	if spec.CriteriaRef != "" {
		return p.Scales[spec.CriteriaRef]
	}
	return nil
}

// MinConfidence is the confidence below which a verdict escalates toward review.
func (p *Policy) MinConfidence() float64 { return floatOr(p.Defaults["min_confidence"], 0.65) }

// OnLowConfidence is what to do with a low-confidence answer; "escalate" is the default.
func (p *Policy) OnLowConfidence() string {
	if s, ok := p.Defaults["on_low_confidence"]; ok && s != nil {
		return pyStr(s)
	}
	return "escalate"
}

// FailClosed reports whether an unreachable Jev blocks on this surface.
//
// Set per surface, because the two are not the same risk: the input check sits in front of a
// model that has its own safety, so failing open there degrades gracefully, while the output
// check is the last line and has nothing behind it.
func (p *Policy) FailClosed(surface Surface) bool {
	setting, ok := p.Defaults["on_error"]
	if !ok || setting == nil {
		return true
	}
	if perSurface, isMap := setting.(map[string]any); isMap {
		value, found := perSurface[string(surface)]
		if !found {
			return true
		}
		setting = value
	}
	return pyStr(setting) == "fail_closed"
}

// ErrorAction is the action a fail-closed surface takes when Jev is unreachable.
func (p *Policy) ErrorAction() Action {
	if s, ok := p.Defaults["error_action"].(string); ok {
		return Action(s)
	}
	return Review
}

// JSON returns the pack as JSON, as it was loaded.
func (p *Policy) JSON() ([]byte, error) { return json.Marshal(p.raw) }

func (p *Policy) String() string {
	return fmt.Sprintf("<Policy %s v%s: %d categories>", p.ID, p.Version, len(p.Categories))
}

// -- parsing --------------------------------------------------------------

func fromRaw(raw map[string]json.RawMessage) (*Policy, error) {
	var top map[string]any
	encoded, _ := json.Marshal(raw)
	if err := json.Unmarshal(encoded, &top); err != nil {
		return nil, err
	}

	p := &Policy{
		ID:          stringOr(top["id"], "custom"),
		Version:     stringOr(top["version"], "0"),
		ContentNote: stringOr(top["content_note"], ""),
		Defaults:    mapOf(top["defaults"]),
		Signals:     map[string]*Signal{},
		Routes:      map[string]string{},
		Categories:  map[string]*Category{},
		Scales:      map[string]json.RawMessage{},
		raw:         raw,
	}
	p.Name = stringOr(top["name"], p.ID)
	for k, v := range mapOf(top["routes"]) {
		p.Routes[k] = pyStr(v)
	}
	for key, value := range raw {
		if strings.HasSuffix(key, "_scale") {
			p.Scales[key] = value
		}
	}

	var sections struct {
		Categories map[string]json.RawMessage `json:"categories"`
		Signals    map[string]json.RawMessage `json:"signals"`
	}
	if err := json.Unmarshal(encoded, &sections); err != nil {
		return nil, fmt.Errorf("policy pack: %w", err)
	}
	for id, body := range sections.Categories {
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			return nil, fmt.Errorf("category %q: %w", id, err)
		}
		cat, err := parseCategory(id, m)
		if err != nil {
			return nil, err
		}
		p.Categories[id] = cat
	}
	for name, body := range sections.Signals {
		sig, err := parseSignal(name, body)
		if err != nil {
			return nil, err
		}
		p.Signals[name] = sig
	}
	if rules, ok := top["rules"].([]any); ok {
		for _, r := range rules {
			p.Rules = append(p.Rules, parseRule(mapOf(r)))
		}
	}
	return p, validate(p)
}

func parseCategory(id string, m map[string]any) (*Category, error) {
	c := &Category{
		ID:                   id,
		Name:                 stringOr(m["name"], id),
		Description:          stringOr(m["description"], ""),
		Refs:                 stringsOf(m["refs"]),
		Surfaces:             map[Surface]bool{},
		Weight:               floatOr(m["weight"], 0.5),
		BaseSeverity:         intOr(m["base_severity"], 2),
		Sentinel:             m["sentinel"] == true,
		SentinelInstructions: stringOr(m["sentinel_instructions"], ""),
		Thresholds:           map[string]Bands{},
		Route:                Route(stringOr(m["route"], "")),
		NeverBelow:           Action(stringOr(m["never_below"], "")),
		Enabled:              m["enabled"] != false,
	}
	surfaces := stringsOf(m["surfaces"])
	if len(surfaces) == 0 {
		surfaces = []string{"input", "output", "conversation"}
	}
	for _, s := range surfaces {
		c.Surfaces[Surface(s)] = true
	}
	for surface, v := range mapOf(m["thresholds"]) {
		bands := mapOf(v)
		var missing []string
		for _, name := range []string{"block", "flag", "review"} {
			if _, ok := toFloat(bands[name]); !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("category %q thresholds[%s] missing %v", id, surface, missing)
		}
		c.Thresholds[surface] = Bands{
			Block:  floatOr(bands["block"], 0),
			Review: floatOr(bands["review"], 0),
			Flag:   floatOr(bands["flag"], 0),
		}
	}
	return c, nil
}

func parseSignal(name string, body json.RawMessage) (*Signal, error) {
	var s struct {
		Type            string          `json:"type"`
		Surfaces        []string        `json:"surfaces"`
		Instructions    string          `json:"instructions"`
		RequiresContext bool            `json:"requires_context"`
		CriteriaRef     string          `json:"criteria_ref"`
		Criteria        json.RawMessage `json:"criteria"`
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("signal %q: %w", name, err)
	}
	var probe map[string]json.RawMessage
	_ = json.Unmarshal(body, &probe)
	_, has := probe["criteria"]
	return &Signal{
		Name:            name,
		Type:            s.Type,
		Surfaces:        s.Surfaces,
		Instructions:    s.Instructions,
		RequiresContext: s.RequiresContext,
		CriteriaRef:     s.CriteriaRef,
		Criteria:        s.Criteria,
		hasCriteria:     has,
	}, nil
}

func parseRule(m map[string]any) Rule {
	when, then := mapOf(m["when"]), mapOf(m["then"])
	r := Rule{
		ID:               stringOr(m["id"], "rule"),
		Signal:           stringOr(when["signal"], ""),
		Op:               stringOr(when["op"], "=="),
		Value:            when["value"],
		ExceptCategories: map[string]bool{},
		Upgrade:          intOr(then["upgrade"], 0),
		Downgrade:        intOr(then["downgrade"], 0),
		CapAction:        Action(stringOr(then["cap_action"], "")),
		FloorAction:      Action(stringOr(then["floor_action"], "")),
		AddFinding:       stringOr(then["add_finding"], ""),
	}
	for _, c := range stringsOf(m["except_categories"]) {
		r.ExceptCategories[c] = true
	}
	return r
}

func validate(p *Policy) error {
	if len(p.Categories) == 0 {
		return fmt.Errorf("policy pack has no categories")
	}
	for id, cat := range p.Categories {
		_, hasInput := cat.Threshold(SurfaceInput)
		if _, hasDefault := cat.Thresholds["default"]; !hasInput && !hasDefault {
			return fmt.Errorf("category %q has no default thresholds", id)
		}
		for surface, b := range cat.Thresholds {
			if !(b.Block >= b.Review && b.Review >= b.Flag) {
				return fmt.Errorf("category %q thresholds[%s] are not ordered block >= review >= flag", id, surface)
			}
		}
	}
	for _, rule := range p.Rules {
		if rule.Signal != "" {
			if _, ok := p.Signals[rule.Signal]; !ok {
				return fmt.Errorf("rule %q refers to unknown signal %q", rule.ID, rule.Signal)
			}
		}
		if rule.AddFinding != "" {
			if _, ok := p.Categories[rule.AddFinding]; !ok {
				return fmt.Errorf("rule %q adds unknown category %q", rule.ID, rule.AddFinding)
			}
		}
	}
	return nil
}

func stringOr(v any, fallback string) string {
	if v == nil {
		return fallback
	}
	return pyStr(v)
}

func mapOf(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func stringsOf(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, pyStr(item))
	}
	return out
}
