// Package prompt renders the material a participant receives: its role, the
// shared context, the visible (or frozen) discussion and the task. The
// output is provider-neutral text; adapters decide how to deliver it.
package prompt

import (
	"fmt"
	"strings"

	"agent-forum/internal/domain"
)

// Material is everything that goes into one prompt.
type Material struct {
	// Self is the participant being prompted.
	Self domain.Participant
	// Participants resolves author names for the transcript.
	Participants map[domain.ID]domain.Participant
	// Context is the shared context in force, nil when the topic has none.
	Context *domain.ContextRevision
	// Messages is the discussion the participant may see, oldest first.
	Messages []domain.Message
	// Task is the request for this run.
	Task string
	// Independent marks a run whose answer must not depend on other
	// participants' current answers (it cannot see them anyway; the note
	// keeps the model from guessing).
	Independent bool
}

// Output is the rendered prompt.
type Output struct {
	// Instructions describe the role and the answer format. Adapters pass
	// them as a system prompt addition where possible.
	Instructions string
	// Prompt holds context, discussion and task in reading order.
	Prompt string
}

// DefaultMaxWords is used when a participant has no word limit of its own.
const DefaultMaxWords = 150

// Build renders the material.
func Build(m Material) Output {
	var ins strings.Builder
	fmt.Fprintf(&ins, "You are %q, a participant in a forum where a human and several AI agents discuss and decide together.\n", m.Self.Name)
	if role := strings.TrimSpace(m.Self.Role); role != "" {
		ins.WriteString("\nYour role:\n")
		ins.WriteString(role)
		ins.WriteString("\n")
	}
	maxWords := m.Self.MaxWords
	if maxWords <= 0 {
		maxWords = DefaultMaxWords
	}
	fmt.Fprintf(&ins, "\nAnswer format: at most %d words of prose. Put code or long material in fenced blocks and keep the explanation short; details are given on request. Reply in the language of the task. Do not repeat the shared context back.\n", maxWords)
	if m.Independent {
		ins.WriteString("\nThis is an independent round: other participants answer the same task at the same time and you cannot see their answers. Do not speculate about them.\n")
	}

	var p strings.Builder
	if m.Context != nil && !m.Context.Doc.IsEmpty() {
		fmt.Fprintf(&p, "# Shared context (revision %d)\n\n", m.Context.Version)
		renderContext(&p, m.Context.Doc)
		p.WriteString("\n")
	}
	if len(m.Messages) > 0 {
		p.WriteString("# Discussion so far\n\n")
		for _, msg := range m.Messages {
			fmt.Fprintf(&p, "[%s]\n%s\n\n", authorName(m, msg), strings.TrimSpace(msg.Body))
		}
	}
	p.WriteString("# Task\n\n")
	p.WriteString(strings.TrimSpace(m.Task))
	p.WriteString("\n")

	return Output{Instructions: ins.String(), Prompt: p.String()}
}

func authorName(m Material, msg domain.Message) string {
	switch msg.AuthorKind {
	case domain.AuthorHuman:
		return "Human"
	case domain.AuthorSystem:
		return "System"
	}
	if p, ok := m.Participants[msg.ParticipantID]; ok {
		return p.Name
	}
	return "Agent"
}

func renderContext(b *strings.Builder, d domain.ContextDoc) {
	if d.Goal != "" {
		fmt.Fprintf(b, "Goal: %s\n", d.Goal)
	}
	list(b, "Constraints", d.Constraints)
	if len(d.Decisions) > 0 {
		b.WriteString("Decisions:\n")
		for _, dec := range d.Decisions {
			if dec.Reason != "" {
				fmt.Fprintf(b, "- %s (because: %s)\n", dec.Text, dec.Reason)
			} else {
				fmt.Fprintf(b, "- %s\n", dec.Text)
			}
		}
	}
	list(b, "Done", d.Done)
	if d.Current != "" {
		fmt.Fprintf(b, "Where we stopped: %s\n", d.Current)
	}
	list(b, "Open questions", d.OpenQuestions)
	if len(d.References) > 0 {
		b.WriteString("References:\n")
		for _, r := range d.References {
			fmt.Fprintf(b, "- %s\n", r.Label)
		}
	}
}

func list(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(title + ":\n")
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
}
