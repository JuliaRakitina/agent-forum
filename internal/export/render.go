package export

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JuliaRakitina/agent-forum/internal/domain"
)

// File is a rendered document ready to be written.
type File struct {
	Name    string
	Content []byte
}

// Render renders a document in the given format.
func Render(doc Document, f Format) (File, error) {
	switch f {
	case JSON:
		raw, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return File{}, fmt.Errorf("export: %w", err)
		}
		return File{Name: Filename(doc, f), Content: append(raw, '\n')}, nil
	case Markdown:
		return File{Name: Filename(doc, f), Content: []byte(RenderMarkdown(doc))}, nil
	}
	return File{}, fmt.Errorf("export: unknown format %q", f)
}

const stamp = "2006-01-02 15:04 UTC"

// RenderMarkdown renders a document as Markdown. Message bodies are
// Markdown already and are included unchanged.
func RenderMarkdown(doc Document) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", doc.Topic.Title)
	fmt.Fprintf(&b, "%s · exported %s · %s\n", doc.Topic.Kind, doc.ExportedAt.UTC().Format(stamp), doc.Format)
	if doc.Omitted.SealedMessages > 0 {
		fmt.Fprintf(&b, "\n> %d sealed %s from unrevealed rounds %s not included.\n",
			doc.Omitted.SealedMessages, plural(doc.Omitted.SealedMessages, "answer", "answers"), plural(doc.Omitted.SealedMessages, "is", "are"))
	}

	if len(doc.Participants) > 0 {
		b.WriteString("\n## Participants\n\n")
		for _, p := range doc.Participants {
			if p.Model != "" {
				fmt.Fprintf(&b, "- %s (%s, %s)\n", p.Name, p.Provider, p.Model)
			} else {
				fmt.Fprintf(&b, "- %s (%s)\n", p.Name, p.Provider)
			}
		}
	}

	if doc.Context != nil && !doc.Context.Doc.IsEmpty() {
		fmt.Fprintf(&b, "\n## Shared context (revision %d)\n\n", doc.Context.Version)
		renderContext(&b, doc.Context.Doc)
	}

	b.WriteString("\n## Discussion\n")
	if len(doc.Messages) == 0 {
		b.WriteString("\nNo messages.\n")
	}
	kinds := map[domain.ID]domain.RoundKind{}
	for _, r := range doc.Rounds {
		kinds[r.ID] = r.Kind
	}
	byID := map[domain.ID]Message{}
	for _, m := range doc.Messages {
		byID[m.ID] = m
	}
	for _, m := range doc.Messages {
		fmt.Fprintf(&b, "\n### %s · %s", m.Author, m.CreatedAt.UTC().Format(stamp))
		if k, ok := kinds[m.RoundID]; ok && k != domain.RoundSingle {
			fmt.Fprintf(&b, " · %s round", k)
		}
		b.WriteString("\n\n")
		if parent, ok := byID[m.ParentID]; ok {
			fmt.Fprintf(&b, "_In reply to %s, %s._\n\n", parent.Author, parent.CreatedAt.UTC().Format(stamp))
		}
		if orig, ok := byID[m.CorrectsID]; ok {
			fmt.Fprintf(&b, "_Corrects the message by %s, %s._\n\n", orig.Author, orig.CreatedAt.UTC().Format(stamp))
		}
		b.WriteString(strings.TrimSpace(m.Body))
		b.WriteString("\n")
	}

	if len(doc.Rounds) > 0 {
		b.WriteString("\n## Rounds\n\n")
		b.WriteString("| Started | Kind | Status | Runs |\n| --- | --- | --- | --- |\n")
		for _, r := range doc.Rounds {
			runs := make([]string, 0, len(r.Runs))
			for _, run := range r.Runs {
				runs = append(runs, run.Participant+": "+string(run.Status))
			}
			status := string(r.Status)
			if r.Sealed {
				status += " (sealed)"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", r.CreatedAt.UTC().Format(stamp), r.Kind, status, cell(strings.Join(runs, ", ")))
		}
	}
	return b.String()
}

func renderContext(b *strings.Builder, d domain.ContextDoc) {
	if d.Goal != "" {
		fmt.Fprintf(b, "**Goal:** %s\n\n", d.Goal)
	}
	list(b, "Constraints", d.Constraints)
	if len(d.Decisions) > 0 {
		b.WriteString("**Decisions**\n\n")
		for _, dec := range d.Decisions {
			if dec.Reason != "" {
				fmt.Fprintf(b, "- %s. Reason: %s\n", strings.TrimSuffix(dec.Text, "."), dec.Reason)
			} else {
				fmt.Fprintf(b, "- %s\n", dec.Text)
			}
		}
		b.WriteString("\n")
	}
	list(b, "Done", d.Done)
	if d.Current != "" {
		fmt.Fprintf(b, "**Where we stopped:** %s\n\n", d.Current)
	}
	list(b, "Open questions", d.OpenQuestions)
	if len(d.References) > 0 {
		refs := make([]string, 0, len(d.References))
		for _, r := range d.References {
			refs = append(refs, r.Label)
		}
		list(b, "References", refs)
	}
}

func list(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "**%s**\n\n", title)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
	b.WriteString("\n")
}

// cell makes text safe inside a Markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.Join(strings.Fields(s), " ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
