package domain

import "fmt"

// RunStatus is the lifecycle state of a run.
type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
	RunTimedOut  RunStatus = "timed_out"
	// RunInterrupted marks a run that was queued or running when the
	// application stopped. It is never restarted automatically, because a
	// restart would repeat a paid call.
	RunInterrupted RunStatus = "interrupted"
)

// Terminal reports whether no further transition is possible.
func (s RunStatus) Terminal() bool {
	switch s {
	case RunSucceeded, RunFailed, RunCancelled, RunTimedOut, RunInterrupted:
		return true
	}
	return false
}

var runTransitions = map[RunStatus][]RunStatus{
	RunQueued:  {RunRunning, RunCancelled, RunInterrupted},
	RunRunning: {RunSucceeded, RunFailed, RunCancelled, RunTimedOut, RunInterrupted},
}

// CanTransition reports whether a run may move from s to next.
func (s RunStatus) CanTransition(next RunStatus) bool {
	for _, allowed := range runTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// Transition returns the next status or an error naming the illegal move.
func (s RunStatus) Transition(next RunStatus) (RunStatus, error) {
	if !s.CanTransition(next) {
		return s, &TransitionError{Entity: "run", From: string(s), To: string(next)}
	}
	return next, nil
}

// RoundStatus is the lifecycle state of a round.
type RoundStatus string

const (
	// RoundRunning: runs are queued or in progress.
	RoundRunning RoundStatus = "running"
	// RoundAwaitingReveal: every run succeeded and the results are sealed
	// until a reveal. Only independent rounds use this state.
	RoundAwaitingReveal RoundStatus = "awaiting_reveal"
	// RoundRevealed: results are visible; an evaluation round may follow.
	RoundRevealed RoundStatus = "revealed"
	// RoundPartial: at least one run did not succeed. The human chooses to
	// retry, cancel or reveal the partial results explicitly.
	RoundPartial RoundStatus = "partial"
	// RoundCancelled: the human stopped the round.
	RoundCancelled RoundStatus = "cancelled"
	// RoundInterrupted: the application stopped while the round was running.
	RoundInterrupted RoundStatus = "interrupted"
)

// Terminal reports whether no further transition is possible.
func (s RoundStatus) Terminal() bool {
	switch s {
	case RoundRevealed, RoundCancelled, RoundInterrupted:
		return true
	}
	return false
}

var roundTransitions = map[RoundStatus][]RoundStatus{
	RoundRunning:        {RoundAwaitingReveal, RoundRevealed, RoundPartial, RoundCancelled, RoundInterrupted},
	RoundAwaitingReveal: {RoundRevealed, RoundCancelled},
	RoundPartial:        {RoundRunning, RoundRevealed, RoundCancelled},
}

// CanTransition reports whether a round may move from s to next.
func (s RoundStatus) CanTransition(next RoundStatus) bool {
	for _, allowed := range roundTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// Transition returns the next status or an error naming the illegal move.
func (s RoundStatus) Transition(next RoundStatus) (RoundStatus, error) {
	if !s.CanTransition(next) {
		return s, &TransitionError{Entity: "round", From: string(s), To: string(next)}
	}
	return next, nil
}

// TransitionError reports an illegal state change.
type TransitionError struct {
	Entity string
	From   string
	To     string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("%s: illegal transition %s -> %s", e.Entity, e.From, e.To)
}
