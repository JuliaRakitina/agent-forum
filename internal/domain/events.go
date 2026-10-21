package domain

import (
	"encoding/json"
	"time"
)

// EventType names a domain event. Event payloads are documented in
// docs/CONTRACT.md and must stay backward compatible: add fields, never
// rename them.
type EventType string

const (
	EventTopicCreated       EventType = "topic.created"
	EventMessageCreated     EventType = "message.created"
	EventMessageRevealed    EventType = "message.revealed"
	EventRunStatusChanged   EventType = "run.status_changed"
	EventRunOutput          EventType = "run.output"
	EventRunToolActivity    EventType = "run.tool_activity"
	EventRoundStatusChanged EventType = "round.status_changed"
	EventContextRevised     EventType = "context.revised"
	EventParticipantChanged EventType = "participant.changed"
)

// Event is the envelope persisted in the event log and delivered to the UI.
// Seq is assigned by storage and is strictly increasing per workspace, so a
// subscriber that reconnects can ask for everything after the last Seq it
// saw.
type Event struct {
	Seq     int64           `json:"seq"`
	Type    EventType       `json:"type"`
	TopicID ID              `json:"topicId,omitempty"`
	At      time.Time       `json:"at"`
	Payload json.RawMessage `json:"payload"`
}

// NewEvent builds an event with a JSON-encoded payload. It panics only on a
// payload that cannot be marshalled, which is a programming error.
func NewEvent(t EventType, topicID ID, payload any) Event {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic("domain: marshal event payload: " + err.Error())
	}
	return Event{Type: t, TopicID: topicID, At: time.Now().UTC(), Payload: raw}
}

// RunStatusPayload is the payload of EventRunStatusChanged.
type RunStatusPayload struct {
	RunID         ID        `json:"runId"`
	ParticipantID ID        `json:"participantId"`
	RoundID       ID        `json:"roundId,omitempty"`
	Status        RunStatus `json:"status"`
	Error         string    `json:"error,omitempty"`
	Usage         *Usage    `json:"usage,omitempty"`
}

// RunOutputPayload is the payload of EventRunOutput: a chunk of streamed
// text. Chunks are coalesced by the orchestrator, so one event may carry
// several model tokens. Index numbers chunks per run from zero.
type RunOutputPayload struct {
	RunID ID     `json:"runId"`
	Index int    `json:"index"`
	Text  string `json:"text"`
	// Sealed chunks belong to an unrevealed independent round; the UI shows
	// progress for them but not the text.
	Sealed bool `json:"sealed"`
}

// RunToolActivityPayload is the payload of EventRunToolActivity. It is a
// summary for the UI, never the tool's raw input or output.
type RunToolActivityPayload struct {
	RunID   ID     `json:"runId"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

// RoundStatusPayload is the payload of EventRoundStatusChanged.
type RoundStatusPayload struct {
	RoundID ID          `json:"roundId"`
	Status  RoundStatus `json:"status"`
}

// MessagePayload is the payload of EventMessageCreated and
// EventMessageRevealed. A sealed message is delivered without its body.
type MessagePayload struct {
	Message Message `json:"message"`
}

// ContextRevisedPayload is the payload of EventContextRevised.
type ContextRevisedPayload struct {
	Revision ContextRevision `json:"revision"`
}

// ParticipantPayload is the payload of EventParticipantChanged.
type ParticipantPayload struct {
	Participant Participant `json:"participant"`
}

// TopicPayload is the payload of EventTopicCreated.
type TopicPayload struct {
	Topic Topic `json:"topic"`
}
