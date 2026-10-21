// ForumApi backed by the Wails bindings. This file is the only place where
// generated types are cast to the UI's narrowed types.
import * as App from '../../wailsjs/go/main/App'
import { domain } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime'
import type { ForumApi, Event } from './types'

/** The generated bindings return `string` where the UI uses literal unions; the JSON is the same. */
function narrow<T>(promise: Promise<unknown>): Promise<T> {
  return promise as Promise<T>
}

export const wailsApi: ForumApi = {
  mode: 'wails',
  capabilities: () => narrow(App.Capabilities()),
  ensureWorkspace: () => narrow(App.EnsureWorkspace()),
  listWorkspaces: () => narrow(App.ListWorkspaces()),
  createWorkspace: (name, rootDir) => narrow(App.CreateWorkspace(name, rootDir)),
  listParticipants: (workspaceId) => narrow(App.ListParticipants(workspaceId)),
  saveParticipant: (participant) => narrow(App.SaveParticipant(participant)),
  listTopics: (workspaceId) => narrow(App.ListTopics(workspaceId)),
  createTopic: (workspaceId, title, kind) => narrow(App.CreateTopic(workspaceId, title, kind)),
  getThread: (topicId, limit) => narrow(App.GetThread(topicId, limit)),
  postMessage: (input) => narrow(App.PostMessage(input)),
  cancelRun: (runId) => App.CancelRun(runId),
  cancelRound: (roundId) => App.CancelRound(roundId),
  revealRound: (roundId) => App.RevealRound(roundId),
  retryRound: (roundId) => App.RetryRound(roundId),
  runOutput: (runId) => App.RunOutput(runId),
  saveContext: (topicId, doc) => narrow(App.SaveContext(topicId, domain.ContextDoc.createFrom(doc))),
  listContextRevisions: (topicId) => narrow(App.ListContextRevisions(topicId)),
  setContextRevisionStatus: (revisionId, status) => App.SetContextRevisionStatus(revisionId, status),
  events: (after, limit) => narrow(App.Events(after, limit)),
  // The runtime delivers the Go struct as JSON; `payload` arrives as an object.
  onEvent: (cb) => EventsOn('forum:event', (raw: unknown) => cb(raw as Event)),
  onResync: (cb) => EventsOn('forum:resync', () => cb()),
}
