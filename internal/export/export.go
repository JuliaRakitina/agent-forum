// Package export renders one topic as a self-contained document: Markdown
// for people, JSON for programs. An export holds what the UI shows: the
// visible discussion, the shared context with its revisions, the
// participants and the history of rounds and runs.
//
// It never holds sealed messages of unrevealed rounds (only their count),
// run errors of such rounds, participant role instructions, local paths or
// the CLIs' native session IDs.
package export

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
	"github.com/JuliaRakitina/agent-forum/internal/storage"
)

// FormatVersion identifies the JSON document layout. Change it when a
// field is removed or changes meaning; adding fields keeps it.
const FormatVersion = "agent-forum.topic/v1"

// Format is an output format.
type Format string

const (
	Markdown Format = "markdown"
	JSON     Format = "json"
)

// ParseFormat accepts "markdown", "md" and "json".
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "markdown", "md", "":
		return Markdown, nil
	case "json":
		return JSON, nil
	}
	return "", fmt.Errorf("export: unknown format %q (use markdown or json)", s)
}

// Document is the exported topic.
type Document struct {
	Format       string        `json:"format"`
	ExportedAt   time.Time     `json:"exportedAt"`
	Topic        domain.Topic  `json:"topic"`
	Participants []Participant `json:"participants"`
	// Context is the revision in force, nil when the topic has none.
	Context   *Revision  `json:"context"`
	Revisions []Revision `json:"revisions"`
	Messages  []Message  `json:"messages"`
	Rounds    []Round    `json:"rounds"`
	Omitted   Omitted    `json:"omitted"`
}

// Participant is a participant without its role instructions.
type Participant struct {
	ID       domain.ID       `json:"id"`
	Name     string          `json:"name"`
	Provider domain.Provider `json:"provider"`
	Model    string          `json:"model,omitempty"`
}

// Revision is one version of the shared context.
type Revision struct {
	Version    int                   `json:"version"`
	Status     domain.RevisionStatus `json:"status"`
	AuthorKind domain.AuthorKind     `json:"authorKind"`
	Doc        domain.ContextDoc     `json:"doc"`
	CreatedAt  time.Time             `json:"createdAt"`
}

// Message is a visible message with its author resolved to a name.
type Message struct {
	ID            domain.ID         `json:"id"`
	ParentID      domain.ID         `json:"parentId,omitempty"`
	CorrectsID    domain.ID         `json:"correctsId,omitempty"`
	AuthorKind    domain.AuthorKind `json:"authorKind"`
	ParticipantID domain.ID         `json:"participantId,omitempty"`
	Author        string            `json:"author"`
	Body          string            `json:"body"`
	RoundID       domain.ID         `json:"roundId,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
}

// Round is a round with its runs.
type Round struct {
	ID           domain.ID          `json:"id"`
	Kind         domain.RoundKind   `json:"kind"`
	Status       domain.RoundStatus `json:"status"`
	Prompt       string             `json:"prompt"`
	Participants []string           `json:"participants"`
	// Sealed is true for an independent round that was not revealed; its
	// answers and run errors are not part of the export.
	Sealed     bool      `json:"sealed"`
	CreatedAt  time.Time `json:"createdAt"`
	RevealedAt time.Time `json:"revealedAt,omitempty"`
	Runs       []Run     `json:"runs"`
}

// Run is one run of a round.
type Run struct {
	ID          domain.ID        `json:"id"`
	Participant string           `json:"participant"`
	Status      domain.RunStatus `json:"status"`
	Error       string           `json:"error,omitempty"`
	Usage       domain.Usage     `json:"usage"`
	StartedAt   time.Time        `json:"startedAt,omitempty"`
	FinishedAt  time.Time        `json:"finishedAt,omitempty"`
}

// Omitted counts what was left out on purpose.
type Omitted struct {
	SealedMessages int `json:"sealedMessages"`
}

// Build reads a topic in one transaction and assembles its document.
func Build(tx *storage.Tx, topicID domain.ID, now time.Time) (Document, error) {
	tp, err := tx.GetTopic(topicID)
	if err != nil {
		return Document{}, err
	}
	doc := Document{Format: FormatVersion, ExportedAt: now.UTC(), Topic: tp,
		Participants: []Participant{}, Revisions: []Revision{}, Messages: []Message{}, Rounds: []Round{}}

	parts, err := tx.ListParticipants(tp.WorkspaceID)
	if err != nil {
		return Document{}, err
	}
	names := map[domain.ID]string{}
	for _, p := range parts {
		names[p.ID] = p.Name
		if p.Provider == domain.ProviderHuman {
			continue
		}
		doc.Participants = append(doc.Participants, Participant{ID: p.ID, Name: p.Name, Provider: p.Provider, Model: p.Model})
	}

	revs, err := tx.ListContextRevisions(topicID)
	if err != nil {
		return Document{}, err
	}
	for _, r := range revs {
		doc.Revisions = append(doc.Revisions, Revision{Version: r.Version, Status: r.Status, AuthorKind: r.AuthorKind, Doc: r.Doc, CreatedAt: r.CreatedAt})
	}
	cur, err := tx.CurrentContext(topicID)
	switch {
	case err == nil:
		doc.Context = &Revision{Version: cur.Version, Status: cur.Status, AuthorKind: cur.AuthorKind, Doc: cur.Doc, CreatedAt: cur.CreatedAt}
	case !errors.Is(err, storage.ErrNotFound):
		return Document{}, err
	}

	visible, err := tx.ListMessages(storage.MessageQuery{TopicID: topicID})
	if err != nil {
		return Document{}, err
	}
	all, err := tx.ListMessages(storage.MessageQuery{TopicID: topicID, IncludeSealed: true})
	if err != nil {
		return Document{}, err
	}
	doc.Omitted.SealedMessages = len(all) - len(visible)
	for _, m := range visible {
		doc.Messages = append(doc.Messages, Message{
			ID: m.ID, ParentID: m.ParentID, CorrectsID: m.CorrectsID, AuthorKind: m.AuthorKind, ParticipantID: m.ParticipantID,
			Author: authorName(m, names), Body: m.Body, RoundID: m.RoundID, CreatedAt: m.CreatedAt,
		})
	}

	rounds, err := tx.ListRoundsByTopic(topicID)
	if err != nil {
		return Document{}, err
	}
	runs, err := tx.ListRunsByTopic(topicID)
	if err != nil {
		return Document{}, err
	}
	byRound := map[domain.ID][]domain.Run{}
	for _, r := range runs {
		byRound[r.RoundID] = append(byRound[r.RoundID], r)
	}
	for _, rd := range rounds {
		sealed := rd.Kind == domain.RoundIndependent && rd.Status != domain.RoundRevealed
		out := Round{ID: rd.ID, Kind: rd.Kind, Status: rd.Status, Prompt: rd.Prompt, Participants: []string{},
			Sealed: sealed, CreatedAt: rd.CreatedAt, RevealedAt: rd.RevealedAt, Runs: []Run{}}
		for _, pid := range rd.ParticipantIDs {
			out.Participants = append(out.Participants, nameOr(names, pid))
		}
		for _, r := range byRound[rd.ID] {
			run := Run{ID: r.ID, Participant: nameOr(names, r.ParticipantID), Status: r.Status, Usage: r.Usage, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt}
			if !sealed {
				// Errors of a sealed run may quote its output.
				run.Error = r.Error
			}
			out.Runs = append(out.Runs, run)
		}
		doc.Rounds = append(doc.Rounds, out)
	}
	return doc, nil
}

func authorName(m domain.Message, names map[domain.ID]string) string {
	switch m.AuthorKind {
	case domain.AuthorHuman:
		return "Human"
	case domain.AuthorSystem:
		return "System"
	}
	return nameOr(names, m.ParticipantID)
}

func nameOr(names map[domain.ID]string, id domain.ID) string {
	if n, ok := names[id]; ok {
		return n
	}
	return "Unknown participant"
}

// Filename suggests a file name for a document: the topic title reduced to
// letters, digits and hyphens, the export date and the extension.
func Filename(doc Document, f Format) string {
	var b strings.Builder
	dash := false
	n := 0
	for _, r := range strings.ToLower(doc.Topic.Title) {
		if n >= 60 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			n++
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
			n++
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "topic"
	}
	ext := "md"
	if f == JSON {
		ext = "json"
	}
	return fmt.Sprintf("%s-%s.%s", slug, doc.ExportedAt.Format("2006-01-02"), ext)
}
