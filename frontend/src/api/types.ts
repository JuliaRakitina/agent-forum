// Types the UI works with. They are derived from the generated Wails models
// (frontend/wailsjs/go/models.ts) so a change on the Go side fails the
// typecheck here, and narrowed where the generator emits `string` or `any`:
// enums become string-literal unions and time fields become ISO strings.
import type { app, domain, runner } from '../../wailsjs/go/models'

/** Drops the helper methods the generated classes carry so plain JSON objects satisfy the type. */
type Plain<T> = { [K in keyof T as T[K] extends (...args: never[]) => unknown ? never : K]: T[K] }
type Override<T, U> = Omit<T, keyof U> & U

export type TopicKind = 'discussion' | 'question' | 'task'
export type Provider = 'human' | 'claude' | 'codex' | 'fake'
export type ParticipationMode = 'mentioned' | 'always' | 'manual'
export type RunProfile = 'subscription' | 'api_key'
export type ToolPolicy = 'none' | 'read_only' | 'workspace_write'
export type IsolationLevel = 'none' | 'soft' | 'strict'
export type AuthorKind = 'human' | 'agent' | 'system'
export type Visibility = 'visible' | 'sealed'
export type RunStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'timed_out' | 'interrupted'
export type RoundKind = 'single' | 'joint' | 'independent'
export type RoundStatus = 'running' | 'awaiting_reveal' | 'revealed' | 'partial' | 'cancelled' | 'interrupted'
export type RevisionStatus = 'accepted' | 'proposed' | 'discarded'
export type AskMode = 'one' | 'joint' | 'independent'

export const TERMINAL_RUN_STATUSES: ReadonlySet<RunStatus> = new Set<RunStatus>([
  'succeeded',
  'failed',
  'cancelled',
  'timed_out',
  'interrupted',
])

export function isTerminalRun(status: RunStatus): boolean {
  return TERMINAL_RUN_STATUSES.has(status)
}

export type Workspace = Override<Plain<domain.Workspace>, { createdAt: string }>
export type Topic = Override<Plain<domain.Topic>, { kind: TopicKind; createdAt: string; updatedAt: string }>
export type Participant = Override<
  Plain<domain.Participant>,
  { provider: Provider; mode: ParticipationMode; profile: RunProfile; tools: ToolPolicy }
>
export type Message = Override<
  Plain<domain.Message>,
  { authorKind: AuthorKind; visibility: Visibility; createdAt: string }
>
export type Usage = Plain<domain.Usage>
export type Run = Override<
  Plain<domain.Run>,
  {
    status: RunStatus
    profile: RunProfile | ''
    tools: ToolPolicy | ''
    isolation: IsolationLevel | ''
    usage: Usage
    createdAt: string
    startedAt?: string
    finishedAt?: string
  }
>
export type FrozenInputs = Plain<domain.FrozenInputs>
export type Round = Override<
  Plain<domain.Round>,
  {
    kind: RoundKind
    status: RoundStatus
    inputs: FrozenInputs
    createdAt: string
    revealedAt?: string
    finishedAt?: string
  }
>
export type Decision = Plain<domain.Decision>
export type Reference = Plain<domain.Reference>
export type ContextDoc = Override<Plain<domain.ContextDoc>, { decisions?: Decision[]; references?: Reference[] }>
export type ContextRevision = Override<
  Plain<domain.ContextRevision>,
  { status: RevisionStatus; authorKind: AuthorKind; doc: ContextDoc; createdAt: string }
>
/** `payload` is raw JSON on the wire; the generator types it as a byte array. */
export type Event = Override<Plain<domain.Event>, { type: string; at: string; payload: unknown }>
export type Capabilities = Override<
  Plain<runner.Capabilities>,
  { provider: Provider; profiles: RunProfile[]; maxIsolation: IsolationLevel }
>
export type Thread = Override<
  Plain<app.Thread>,
  {
    topic: Topic
    messages: Message[]
    runs: Run[]
    rounds: Round[]
    participants: Participant[]
    context?: ContextRevision
  }
>
export type PostInput = Override<Plain<app.PostInput>, { mode: AskMode }>
export type PostResult = Override<Plain<app.PostResult>, { message: Message; round?: Round }>

export type ApiMode = 'wails' | 'mock'

/** Everything the UI needs from the backend, in either the Wails window or the browser mock. */
export interface ForumApi {
  readonly mode: ApiMode
  capabilities(): Promise<Capabilities[]>
  ensureWorkspace(): Promise<Workspace>
  listWorkspaces(): Promise<Workspace[]>
  createWorkspace(name: string, rootDir: string): Promise<Workspace>
  listParticipants(workspaceId: string): Promise<Participant[]>
  saveParticipant(participant: Participant): Promise<Participant>
  listTopics(workspaceId: string): Promise<Topic[]>
  createTopic(workspaceId: string, title: string, kind: TopicKind): Promise<Topic>
  getThread(topicId: string, limit: number): Promise<Thread>
  postMessage(input: PostInput): Promise<PostResult>
  cancelRun(runId: string): Promise<void>
  cancelRound(roundId: string): Promise<void>
  revealRound(roundId: string): Promise<void>
  retryRound(roundId: string): Promise<void>
  runOutput(runId: string): Promise<string>
  saveContext(topicId: string, doc: ContextDoc): Promise<ContextRevision>
  listContextRevisions(topicId: string): Promise<ContextRevision[]>
  setContextRevisionStatus(revisionId: string, status: RevisionStatus): Promise<void>
  events(after: number, limit: number): Promise<Event[]>
  /** Subscribes to committed events in sequence order; returns the unsubscribe function. */
  onEvent(cb: (ev: Event) => void): () => void
  /** Fires when events were dropped and the UI should reload from `events(lastSeq)`. */
  onResync(cb: () => void): () => void
}
