package guardrail

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Names used on the wire.
const (
	NoneLabel      = "none"
	Hazard         = "hazard"
	SentinelPrefix = "s_"
)

// Question subsets.
const (
	SubsetFull      = "full"
	SubsetSentinels = "sentinels"
)

var hazardInstructions = map[Surface]string{
	SurfaceInput:        "Which hazard category best describes the most serious safety problem in this user message? Choose 'none' if the message raises no safety problem.",
	SurfaceOutput:       "Which hazard category best describes the most serious safety problem in this assistant reply? Judge the reply itself, not the user message that prompted it. Choose 'none' if the reply raises no safety problem.",
	SurfaceConversation: "Across this whole conversation, which hazard category best describes the most serious safety problem? Choose 'none' if the conversation raises no safety problem.",
}

const noneDescription = "No safety problem: the content raises none of the listed hazards."

// Question is one question in Jev wire form.
type Question struct {
	Type         string          `json:"type"`
	Instructions string          `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}

// Questions is a question set keyed by question name.
type Questions map[string]Question

// State is the content put in front of Jev.
type State map[string]any

// BuildQuestions is the question set for a surface.
//
// Jev answers every question in a request in parallel and charges only for input tokens, so the
// guardrail asks everything it could need in one round trip rather than chaining calls.
//
// SubsetSentinels asks only the yes/no questions for the categories that must not be missed.
// Mid-stream checks use it: the text is incomplete, so the categories that need the whole reply
// to judge would only produce noise, and the 18-label choice is the expensive part to skip.
func BuildQuestions(p *Policy, surface Surface, hasContext bool, subset string) (Questions, error) {
	if subset == "" {
		subset = SubsetFull
	}
	if subset == SubsetSentinels {
		sentinels := p.Sentinels(surface)
		if len(sentinels) == 0 {
			return nil, fmt.Errorf("policy %q has no sentinels for surface %q", p.ID, surface)
		}
		out := Questions{}
		for _, cat := range sentinels {
			out[SentinelPrefix+cat.ID] = Question{Type: "noul", Instructions: cat.SentinelInstructions}
		}
		return out, nil
	}
	if subset != SubsetFull {
		return nil, fmt.Errorf("unknown question subset %q", subset)
	}
	instructions, ok := hazardInstructions[surface]
	if !ok {
		return nil, fmt.Errorf("unknown surface %q", surface)
	}

	categories := p.ForSurface(surface)
	if len(categories) == 0 {
		return nil, fmt.Errorf("policy %q has no categories for surface %q", p.ID, surface)
	}
	if p.ContentNote != "" {
		instructions += " " + p.ContentNote
	}

	// The labels go out in order: "none" first, then the most serious hazard first.
	labels := [][2]string{{NoneLabel, noneDescription}}
	for _, c := range categories {
		labels = append(labels, [2]string{c.ID, c.Description})
	}
	questions := Questions{
		Hazard: {Type: "choice", Instructions: instructions, Criteria: orderedObject(labels)},
	}
	for _, cat := range p.Sentinels(surface) {
		questions[SentinelPrefix+cat.ID] = Question{Type: "noul", Instructions: cat.SentinelInstructions}
	}
	for name, spec := range p.SignalsFor(surface, hasContext) {
		q := Question{Type: spec.Type, Instructions: spec.Instructions}
		if criteria := p.CriteriaFor(spec); len(criteria) > 0 && string(criteria) != "null" {
			q.Criteria = criteria
		}
		questions[name] = q
	}
	return questions, nil
}

// InputState is the state for a user message about to be sent to the model.
func InputState(content string, metadata map[string]any) State {
	state := State{"evaluating": "user_message", "user_message": content}
	if len(metadata) > 0 {
		state["deployment_context"] = copyMap(metadata)
	}
	return state
}

// OutputState is the state for an assistant reply about to be delivered. An empty userMessage is
// left out.
func OutputState(reply, userMessage string, context []string, metadata map[string]any) State {
	state := State{"evaluating": "assistant_reply", "assistant_reply": reply}
	if userMessage != "" {
		state["user_message"] = userMessage
	}
	if len(context) > 0 {
		state["reference_context"] = append([]string(nil), context...)
	}
	if len(metadata) > 0 {
		state["deployment_context"] = copyMap(metadata)
	}
	return state
}

// ConversationState is the state for a whole conversation, used to catch patterns no single turn
// shows.
func ConversationState(turns []Turn, metadata map[string]any) State {
	state := State{"evaluating": "conversation", "turns": append([]Turn{}, turns...)}
	if len(metadata) > 0 {
		state["deployment_context"] = copyMap(metadata)
	}
	return state
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func orderedObject(pairs [][2]string) json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, kv := range pairs {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(marshalNoEscape(kv[0]))
		buf.WriteByte(':')
		buf.Write(marshalNoEscape(kv[1]))
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// marshalNoEscape encodes JSON without escaping <, > and &, as Python's json module does.
func marshalNoEscape(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte("null")
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}
