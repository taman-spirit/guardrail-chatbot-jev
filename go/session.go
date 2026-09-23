package guardrail

// ActionRisk is how much each action contributes to a session's risk score.
var ActionRisk = map[Action]float64{Allow: 0, Flag: 0.25, Review: 0.6, Block: 1.0}

// Session carries risk across turns.
//
// A multi-turn attack is made of turns that are each defensible on their own. Judging every turn
// from a standing start is what makes that work. A session keeps the transcript, accumulates a
// decaying risk score, and raises a floor under the next few turns when it has already seen
// something, so a conversation that has been escalating is not read as if it had just begun.
//
// Not safe for concurrent use; use one per conversation.
type Session struct {
	// ID is the conversation identifier, carried into verdict metadata.
	ID string
	// Decay is how much of the previous risk survives each turn. 0.5 means a single flagged turn
	// stops mattering after three or four clean ones.
	Decay float64
	// CarryTurns is how many following turns a raised floor applies to.
	CarryTurns int
	// MaxTurns is the transcript window kept for conversation checks. The escalation pattern a
	// conversation check looks for lives in the recent turns, so a short window finds it just as
	// well and costs a fraction of the input tokens. Jev's state limit is 32k tokens, which leaves
	// room for roughly 40 turns of ordinary chat; raise it when your conversations genuinely build
	// over more than ten turns.
	MaxTurns int

	Turns    []Turn
	Risk     float64
	Verdicts []Verdict

	floor     Action
	floorLeft int
}

// NewSession starts a session with the default decay 0.5, carry of 2 turns and window of 10.
func NewSession(id string) *Session {
	return &Session{ID: id, Decay: 0.5, CarryTurns: 2, MaxTurns: 10, floor: Allow}
}

// AddTurn appends a message, keeping only the most recent MaxTurns.
func (s *Session) AddTurn(role, content string) {
	s.Turns = append(s.Turns, Turn{Role: role, Content: content})
	if s.MaxTurns > 0 && len(s.Turns) > s.MaxTurns {
		s.Turns = append([]Turn(nil), s.Turns[len(s.Turns)-s.MaxTurns:]...)
	}
}

// Extend appends several turns.
func (s *Session) Extend(turns []Turn) {
	for _, t := range turns {
		s.AddTurn(t.Role, t.Content)
	}
}

// History is a copy of the transcript window.
func (s *Session) History() []Turn { return append([]Turn(nil), s.Turns...) }

// Floor is the minimum action the next check will resolve to.
func (s *Session) Floor() Action {
	if s.floorLeft > 0 && s.floor != "" {
		return s.floor
	}
	return Allow
}

// FloorTurnsLeft is how many more turns the current floor applies to.
func (s *Session) FloorTurnsLeft() int { return s.floorLeft }

// Observe folds a verdict into the session's state.
//
// A degraded verdict is ignored: it reflects an outage, not the conversation.
func (s *Session) Observe(v Verdict) {
	if v.Degraded {
		return
	}
	s.Verdicts = append(s.Verdicts, v)
	s.Risk = max(s.Risk*s.Decay, ActionRisk[v.Action])

	// A conversation-level finding is the one that justifies holding the next turns to a higher
	// standard, because it is about the pattern rather than a single message.
	if v.Surface == SurfaceConversation && Rank(v.Action) >= Rank(Review) {
		s.raise(Review)
	} else if v.Action == Block {
		s.raise(Flag)
	}
}

// Advance lets a raised floor expire. Call it once per completed turn.
func (s *Session) Advance() {
	if s.floorLeft > 0 {
		s.floorLeft--
		if s.floorLeft == 0 {
			s.floor = Allow
		}
	}
}

func (s *Session) raise(floor Action) {
	s.floor = Stronger(s.Floor(), floor)
	s.floorLeft = s.CarryTurns
}

// Metadata is the deployment context worth putting in front of Jev on later turns.
func (s *Session) Metadata() map[string]any {
	return map[string]any{
		"conversation_id": s.ID,
		"turn_number":     len(s.Turns) + 1,
		"session_risk":    round(s.Risk, 3),
	}
}

// AsState is everything a store has to carry for a conversation to survive the process.
//
// The verdict log is deliberately not in here. It is in-process observability, and a restored
// session should not claim to have emitted verdicts this process never saw. What is here is
// exactly what changes a later decision: the transcript, the risk, and the floor.
//
// The keys are the same in every language, so a session written by one can be read by another.
func (s *Session) AsState() map[string]any {
	floor := s.floor
	if floor == "" {
		floor = Allow
	}
	return map[string]any{
		"id":               s.ID,
		"decay":            s.Decay,
		"carry_turns":      s.CarryTurns,
		"max_turns":        s.MaxTurns,
		"turns":            s.History(),
		"risk":             s.Risk,
		"floor":            floor,
		"floor_turns_left": s.floorLeft,
	}
}

// SessionFromState rebuilds a session from AsState, or from JSON of it.
//
// Tolerant of a missing or malformed field, because this state comes back from a store and a
// half-written record should cost one conversation's memory, not the request.
func SessionFromState(state map[string]any) *Session {
	s := &Session{
		ID:         stringOr(state["id"], ""),
		Decay:      floatOr(state["decay"], 0.5),
		CarryTurns: intOr(state["carry_turns"], 2),
		MaxTurns:   intOr(state["max_turns"], 10),
		Risk:       floatOr(state["risk"], 0),
		floor:      Allow,
	}
	switch turns := state["turns"].(type) {
	case []Turn:
		s.Extend(turns)
	case []any:
		for _, t := range turns {
			switch v := t.(type) {
			case map[string]any:
				tt := turnFromMap(v)
				s.AddTurn(tt.Role, tt.Content)
			case []any:
				if len(v) == 2 {
					s.AddTurn(pyStr(v[0]), pyStr(v[1]))
				}
			}
		}
	}
	floor := Action(stringOr(state["floor"], string(Allow)))
	left := intOr(state["floor_turns_left"], 0)
	// A floor that outlived its counter, or a value no longer in the ladder, is no floor.
	if validAction(floor) && floor != Allow && left > 0 {
		s.floor = floor
		s.floorLeft = left
	}
	return s
}

// Summary is a short description of the session, for logs.
func (s *Session) Summary() map[string]any {
	return map[string]any{
		"id":               s.ID,
		"turns":            len(s.Turns),
		"risk":             round(s.Risk, 3),
		"floor":            s.Floor(),
		"floor_turns_left": s.floorLeft,
		"checks":           len(s.Verdicts),
	}
}
