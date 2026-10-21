// Reducer for the open topic. It applies committed events in sequence order
// on top of a GetThread snapshot and asks for a reload when it notices a gap.
import { parseEvent } from '../api/events'
import type { ContextRevision, Event, Message, Round, Run, Thread, Topic } from '../api/types'
import { isTerminalRun } from '../api/types'

export interface StreamBuffer {
  /** Text received so far, in index order. */
  text: string
  /** Index of the next chunk to append. */
  nextIndex: number
  /** Chunks that arrived ahead of nextIndex. */
  pending: Record<number, string>
  /** True for runs of an unrevealed independent round: progress only, no text. */
  sealed: boolean
  /** Number of chunks received, for progress display of sealed runs. */
  chunks: number
}

export interface ThreadState {
  topicId: string | null
  loading: boolean
  error: string | null
  topic: Topic | null
  messages: Message[]
  runs: Run[]
  rounds: Round[]
  context: ContextRevision | null
  streams: Record<string, StreamBuffer>
  /** Last tool activity per run, as a short line for the UI. */
  tools: Record<string, string>
  lastSeq: number
  /** Set when a gap in sequence numbers or an unknown entity was seen. */
  resyncNeeded: boolean
  /** Events that arrived while a snapshot was loading; replayed on success. */
  pending: Event[]
}

export const initialThreadState: ThreadState = {
  topicId: null,
  loading: false,
  error: null,
  topic: null,
  messages: [],
  runs: [],
  rounds: [],
  context: null,
  streams: {},
  tools: {},
  lastSeq: 0,
  resyncNeeded: false,
  pending: [],
}

export type ThreadAction =
  | { type: 'load/start'; topicId: string }
  | { type: 'load/success'; thread: Thread; outputs?: Record<string, string> }
  | { type: 'load/failure'; error: string }
  | { type: 'event'; event: Event }
  | { type: 'resync/request' }
  | { type: 'message/add'; message: Message }
  | { type: 'round/add'; round: Round }

export function threadReducer(state: ThreadState, action: ThreadAction): ThreadState {
  switch (action.type) {
    case 'load/start':
      // A reload of the open topic keeps what is on screen until the
      // snapshot arrives; switching topics starts from scratch.
      if (state.topicId === action.topicId) return { ...state, loading: true, pending: [], resyncNeeded: false }
      return { ...initialThreadState, topicId: action.topicId, loading: true }
    case 'load/failure':
      return { ...state, loading: false, error: action.error, resyncNeeded: false }
    case 'load/success': {
      if (action.thread.topic.id !== state.topicId) return state
      let next: ThreadState = {
        ...state,
        loading: false,
        error: null,
        topic: action.thread.topic,
        messages: [...action.thread.messages],
        runs: [...action.thread.runs],
        rounds: [...action.thread.rounds],
        context: action.thread.context ?? null,
        streams: seedStreams(action.thread, action.outputs ?? {}),
        tools: {},
        lastSeq: action.thread.lastEventSeq,
        resyncNeeded: false,
        pending: [],
      }
      for (const ev of state.pending) next = applyEvent(next, ev)
      return next
    }
    case 'event':
      if (state.loading) return { ...state, pending: [...state.pending, action.event] }
      return applyEvent(state, action.event)
    case 'resync/request':
      return { ...state, resyncNeeded: true }
    case 'message/add':
      if (action.message.topicId !== state.topicId) return state
      return { ...state, messages: upsertMessage(state.messages, action.message) }
    case 'round/add':
      if (action.round.topicId !== state.topicId) return state
      return { ...state, rounds: upsertById(state.rounds, action.round) }
    default:
      return state
  }
}

function seedStreams(thread: Thread, outputs: Record<string, string>): Record<string, StreamBuffer> {
  const streams: Record<string, StreamBuffer> = {}
  const sealedRounds = new Set(thread.rounds.filter((r) => r.kind === 'independent' && r.status !== 'revealed').map((r) => r.id))
  for (const run of thread.runs) {
    if (isTerminalRun(run.status)) continue
    const sealed = run.roundId !== undefined && run.roundId !== '' && sealedRounds.has(run.roundId)
    const text = sealed ? '' : (outputs[run.id] ?? '')
    streams[run.id] = { text, nextIndex: 0, pending: {}, sealed, chunks: 0 }
  }
  return streams
}

function applyEvent(state: ThreadState, ev: Event): ThreadState {
  if (ev.seq <= state.lastSeq) return state
  let next = state
  if (state.lastSeq > 0 && ev.seq > state.lastSeq + 1) {
    next = { ...next, resyncNeeded: true }
  }
  next = { ...next, lastSeq: ev.seq }
  const parsed = parseEvent(ev)
  if (!parsed) return next
  const forThisTopic = ev.topicId !== undefined && ev.topicId === state.topicId
  switch (parsed.type) {
    case 'message.created':
    case 'message.revealed': {
      if (!forThisTopic) return next
      const message = parsed.payload.message
      return { ...next, messages: upsertMessage(next.messages, message) }
    }
    case 'run.status_changed': {
      if (!forThisTopic) return next
      const p = parsed.payload
      const existing = next.runs.find((r) => r.id === p.runId)
      const run: Run = existing
        ? { ...existing, status: p.status, error: p.error, usage: p.usage ?? existing.usage, roundId: p.roundId ?? existing.roundId }
        : {
            id: p.runId,
            topicId: state.topicId ?? '',
            participantId: p.participantId,
            roundId: p.roundId,
            status: p.status,
            profile: '',
            tools: '',
            isolation: '',
            error: p.error,
            usage: p.usage ?? { inputTokens: 0, outputTokens: 0, cachedTokens: 0, costUsd: 0 },
            createdAt: ev.at,
          }
      if (isTerminalRun(run.status)) run.finishedAt = run.finishedAt ?? ev.at
      else if (run.status === 'running') run.startedAt = run.startedAt ?? ev.at
      const streams = { ...next.streams }
      if (!isTerminalRun(run.status) && !streams[run.id]) {
        const round = run.roundId ? next.rounds.find((r) => r.id === run.roundId) : undefined
        const sealed = round !== undefined && round.kind === 'independent' && round.status !== 'revealed'
        streams[run.id] = { text: '', nextIndex: 0, pending: {}, sealed, chunks: 0 }
      }
      return { ...next, runs: upsertById(next.runs, run), streams }
    }
    case 'run.output': {
      if (!forThisTopic) return next
      const p = parsed.payload
      const current = next.streams[p.runId] ?? { text: '', nextIndex: 0, pending: {}, sealed: false, chunks: 0 }
      const updated: StreamBuffer = { ...current, pending: { ...current.pending }, chunks: current.chunks + 1 }
      if (p.sealed) {
        updated.sealed = true
      } else {
        updated.pending[p.index] = p.text
        while (updated.pending[updated.nextIndex] !== undefined) {
          updated.text += updated.pending[updated.nextIndex]
          delete updated.pending[updated.nextIndex]
          updated.nextIndex += 1
        }
      }
      return { ...next, streams: { ...next.streams, [p.runId]: updated } }
    }
    case 'run.tool_activity': {
      if (!forThisTopic) return next
      const p = parsed.payload
      const line = [p.tool, p.status, p.summary].filter((s) => s !== '').join(' · ')
      return { ...next, tools: { ...next.tools, [p.runId]: line } }
    }
    case 'round.status_changed': {
      if (!forThisTopic) return next
      const p = parsed.payload
      const round = next.rounds.find((r) => r.id === p.roundId)
      if (!round) return { ...next, resyncNeeded: true }
      const updated: Round = { ...round, status: p.status }
      if (p.status === 'revealed') updated.revealedAt = updated.revealedAt ?? ev.at
      if (p.status !== 'running' && p.status !== 'awaiting_reveal') updated.finishedAt = updated.finishedAt ?? ev.at
      let streams = next.streams
      if (p.status === 'revealed') {
        streams = { ...streams }
        for (const run of next.runs) {
          if (run.roundId === p.roundId && streams[run.id]) streams[run.id] = { ...streams[run.id], sealed: false }
        }
      }
      return { ...next, rounds: upsertById(next.rounds, updated), streams }
    }
    case 'context.revised': {
      if (!forThisTopic) return next
      const rev = parsed.payload.revision
      if (rev.status === 'accepted' && (!next.context || rev.version >= next.context.version)) {
        return { ...next, context: rev }
      }
      if (next.context && next.context.id === rev.id && rev.status !== 'accepted') {
        return { ...next, resyncNeeded: true }
      }
      return next
    }
    default:
      return next
  }
}

function upsertById<T extends { id: string }>(list: T[], item: T): T[] {
  const idx = list.findIndex((x) => x.id === item.id)
  if (idx < 0) return [...list, item]
  const copy = list.slice()
  copy[idx] = item
  return copy
}

function upsertMessage(list: Message[], message: Message): Message[] {
  const idx = list.findIndex((m) => m.id === message.id)
  if (idx < 0) {
    const copy = [...list, message]
    copy.sort((a, b) => (a.createdAt < b.createdAt ? -1 : a.createdAt > b.createdAt ? 1 : a.id < b.id ? -1 : 1))
    return copy
  }
  const copy = list.slice()
  copy[idx] = message
  return copy
}

/** Selectors used by components. */
export function runsOfRound(state: ThreadState, roundId: string): Run[] {
  return state.runs.filter((r) => r.roundId === roundId)
}

export function messageForRun(state: ThreadState, runId: string): Message | undefined {
  return state.messages.find((m) => m.runId === runId)
}
