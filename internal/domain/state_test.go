package domain

import (
	"errors"
	"testing"
)

func TestRunTransitions(t *testing.T) {
	cases := []struct {
		from, to RunStatus
		ok       bool
	}{
		{RunQueued, RunRunning, true},
		{RunQueued, RunCancelled, true},
		{RunQueued, RunInterrupted, true},
		{RunQueued, RunSucceeded, false},
		{RunRunning, RunSucceeded, true},
		{RunRunning, RunFailed, true},
		{RunRunning, RunTimedOut, true},
		{RunRunning, RunQueued, false},
		{RunSucceeded, RunRunning, false},
		{RunCancelled, RunSucceeded, false},
		{RunInterrupted, RunRunning, false},
	}
	for _, c := range cases {
		got, err := c.from.Transition(c.to)
		if c.ok {
			if err != nil || got != c.to {
				t.Errorf("%s -> %s: want ok, got %v (%s)", c.from, c.to, err, got)
			}
			continue
		}
		var te *TransitionError
		if !errors.As(err, &te) || got != c.from {
			t.Errorf("%s -> %s: want TransitionError and unchanged state, got %v (%s)", c.from, c.to, err, got)
		}
	}
}

func TestRunTerminal(t *testing.T) {
	for _, s := range []RunStatus{RunSucceeded, RunFailed, RunCancelled, RunTimedOut, RunInterrupted} {
		if !s.Terminal() {
			t.Errorf("%s should be terminal", s)
		}
		if len(runTransitions[s]) != 0 {
			t.Errorf("%s is terminal but has transitions", s)
		}
	}
	for _, s := range []RunStatus{RunQueued, RunRunning} {
		if s.Terminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}

func TestRoundTransitions(t *testing.T) {
	cases := []struct {
		from, to RoundStatus
		ok       bool
	}{
		{RoundRunning, RoundAwaitingReveal, true},
		{RoundRunning, RoundRevealed, true},
		{RoundRunning, RoundPartial, true},
		{RoundRunning, RoundCancelled, true},
		{RoundAwaitingReveal, RoundRevealed, true},
		{RoundAwaitingReveal, RoundRunning, false},
		{RoundAwaitingReveal, RoundPartial, false},
		{RoundPartial, RoundRunning, true},
		{RoundPartial, RoundRevealed, true},
		{RoundRevealed, RoundRunning, false},
		{RoundCancelled, RoundRevealed, false},
	}
	for _, c := range cases {
		_, err := c.from.Transition(c.to)
		if (err == nil) != c.ok {
			t.Errorf("%s -> %s: ok=%v, err=%v", c.from, c.to, c.ok, err)
		}
	}
}

func TestEventPayloadRoundTrip(t *testing.T) {
	ev := NewEvent(EventRunStatusChanged, "topic-1", RunStatusPayload{RunID: "run-1", Status: RunRunning})
	if ev.Type != EventRunStatusChanged || ev.TopicID != "topic-1" || ev.At.IsZero() {
		t.Fatalf("unexpected envelope: %+v", ev)
	}
	if string(ev.Payload) != `{"runId":"run-1","participantId":"","status":"running"}` {
		t.Fatalf("unexpected payload: %s", ev.Payload)
	}
}
