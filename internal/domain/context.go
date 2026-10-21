package domain

// ContextDoc is the shared context of a topic: what we are doing, what has
// been decided and why, where we stopped. It is structured so the UI can show
// and pin sections and so adapters can render it for any CLI.
type ContextDoc struct {
	Goal        string     `json:"goal"`
	Constraints []string   `json:"constraints,omitempty"`
	Decisions   []Decision `json:"decisions,omitempty"`
	Done        []string   `json:"done,omitempty"`
	// Current is where the work stopped and what comes next.
	Current       string      `json:"current,omitempty"`
	OpenQuestions []string    `json:"openQuestions,omitempty"`
	References    []Reference `json:"references,omitempty"`
}

// Decision is a confirmed decision with its reason.
type Decision struct {
	Text   string `json:"text"`
	Reason string `json:"reason,omitempty"`
	// MessageID is the message where the decision was made.
	MessageID ID `json:"messageId,omitempty"`
}

// Reference points at a message or artifact version that backs the context.
type Reference struct {
	Label      string `json:"label"`
	MessageID  ID     `json:"messageId,omitempty"`
	ArtifactID ID     `json:"artifactId,omitempty"`
}

// IsEmpty reports whether the document carries no content at all.
func (d ContextDoc) IsEmpty() bool {
	return d.Goal == "" && len(d.Constraints) == 0 && len(d.Decisions) == 0 &&
		len(d.Done) == 0 && d.Current == "" && len(d.OpenQuestions) == 0 &&
		len(d.References) == 0
}
