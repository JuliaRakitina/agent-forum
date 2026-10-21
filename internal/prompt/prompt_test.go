package prompt

import (
	"strings"
	"testing"

	"agent-forum/internal/domain"
)

func TestBuildRendersEverything(t *testing.T) {
	self := domain.Participant{ID: "p1", Name: "Claude", Role: "You review architecture.", MaxWords: 80}
	other := domain.Participant{ID: "p2", Name: "Codex"}
	out := Build(Material{
		Self:         self,
		Participants: map[domain.ID]domain.Participant{"p1": self, "p2": other},
		Context: &domain.ContextRevision{Version: 3, Doc: domain.ContextDoc{
			Goal:        "Ship MVP",
			Constraints: []string{"macOS first"},
			Decisions:   []domain.Decision{{Text: "Wails v2", Reason: "stable"}},
			Current:     "writing the orchestrator",
		}},
		Messages: []domain.Message{
			{AuthorKind: domain.AuthorHuman, Body: "What do you think?"},
			{AuthorKind: domain.AuthorAgent, ParticipantID: "p2", Body: "Looks fine."},
			{AuthorKind: domain.AuthorAgent, ParticipantID: "unknown", Body: "?"},
		},
		Task:        "Give your verdict.",
		Independent: true,
	})
	for _, want := range []string{`You are "Claude"`, "You review architecture.", "at most 80 words", "independent round"} {
		if !strings.Contains(out.Instructions, want) {
			t.Errorf("instructions missing %q:\n%s", want, out.Instructions)
		}
	}
	for _, want := range []string{"# Shared context (revision 3)", "Goal: Ship MVP", "- macOS first", "- Wails v2 (because: stable)", "Where we stopped: writing the orchestrator",
		"# Discussion so far", "[Human]\nWhat do you think?", "[Codex]\nLooks fine.", "[Agent]\n?", "# Task\n\nGive your verdict."} {
		if !strings.Contains(out.Prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, out.Prompt)
		}
	}
	idx := strings.Index(out.Prompt, "# Shared context")
	if idx > strings.Index(out.Prompt, "# Discussion") || strings.Index(out.Prompt, "# Discussion") > strings.Index(out.Prompt, "# Task") {
		t.Error("sections out of order")
	}
}

func TestBuildMinimal(t *testing.T) {
	out := Build(Material{Self: domain.Participant{Name: "X"}, Task: "hi"})
	if strings.Contains(out.Prompt, "Shared context") || strings.Contains(out.Prompt, "Discussion") {
		t.Errorf("empty sections rendered:\n%s", out.Prompt)
	}
	if !strings.Contains(out.Instructions, "at most 150 words") {
		t.Errorf("default word limit missing:\n%s", out.Instructions)
	}
}
