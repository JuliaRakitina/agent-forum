// Package domain holds the entities, state machines and event types of the
// forum. It has no dependencies on storage, CLI adapters or the window
// toolkit: every other package depends on it, it depends on none of them.
//
// Terminology:
//
//   - Workspace: a project folder with its own topics, participants and
//     settings.
//   - Topic: a thread of messages inside a workspace.
//   - Participant: a configured agent (provider + model + role) or the human.
//   - Run: one invocation of a participant's CLI for one message or round.
//   - Round: a coordinated set of runs, for example an "independent answers"
//     round whose results stay sealed until every run has finished.
//   - ContextRevision: a version of the shared context document of a topic.
package domain
