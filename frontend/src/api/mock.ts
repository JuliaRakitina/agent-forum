// In-memory ForumApi for developing the UI in a browser. It mirrors the
// orchestrator's behaviour: rounds, sealed independent answers, streaming
// output in chunks, cancellation, retry and an event log with sequence
// numbers. Nothing here talks to a network or a CLI.
import type {
  AskMode,
  Capabilities,
  ContextDoc,
  ContextRevision,
  Event,
  ForumApi,
  Message,
  Participant,
  PostInput,
  PostResult,
  RevisionStatus,
  Round,
  RoundKind,
  RoundStatus,
  Run,
  RunStatus,
  Thread,
  Topic,
  TopicKind,
  Workspace,
} from './types'
import { isTerminalRun } from './types'

const CHUNK_INTERVAL_MS = 90
const QUEUE_DELAY_MS = 350

let counter = 0
function nextId(prefix: string): string {
  counter += 1
  return `${prefix}-${counter}`
}

function nowIso(offsetMs = 0): string {
  return new Date(Date.now() + offsetMs).toISOString()
}

function clone<T>(value: T): T {
  return structuredClone(value)
}

interface ActiveRun {
  timer: ReturnType<typeof setInterval> | null
  chunks: string[]
  next: number
  text: string
  outputIndex: number
}

const mockCapabilities: Capabilities[] = [
  {
    provider: 'claude',
    streaming: true,
    resume: true,
    cancel: true,
    toolApprovals: false,
    toolRestrictions: true,
    efforts: ['low', 'medium', 'high', 'xhigh', 'max'],
    profiles: ['subscription', 'api_key'],
    maxIsolation: 'soft',
  },
  {
    provider: 'codex',
    streaming: false,
    resume: true,
    cancel: true,
    toolApprovals: false,
    toolRestrictions: true,
    efforts: ['minimal', 'low', 'medium', 'high', 'xhigh'],
    profiles: ['subscription', 'api_key'],
    maxIsolation: 'soft',
  },
  {
    provider: 'fake',
    streaming: true,
    resume: false,
    cancel: true,
    toolApprovals: false,
    toolRestrictions: true,
    efforts: ['low', 'high'],
    profiles: ['subscription', 'api_key'],
    maxIsolation: 'soft',
  },
]

function replyFor(p: Participant, task: string, kind: RoundKind): string {
  const short = task.length > 60 ? `${task.slice(0, 60)}…` : task
  const lead = kind === 'independent' ? 'Independent answer' : 'Reply'
  if (p.provider === 'codex') {
    return [
      `**${lead} from ${p.name}** to "${short}"`,
      '',
      '1. Keep the orchestrator as the only writer of run results.',
      '2. Verify the isolation flags on the real machine before labelling a round as isolated.',
      '',
      '```go',
      'func (s *Service) RevealRound(ctx context.Context, id domain.ID) error {',
      '\treturn s.commit(ctx, func(tx *storage.Tx, emit func(domain.Event)) error {',
      '\t\tround, err := tx.GetRound(id)',
      '\t\tif err != nil {',
      '\t\t\treturn err',
      '\t\t}',
      '\t\treturn s.reveal(tx, emit, round)',
      '\t})',
      '}',
      '```',
      '',
      'Verdict: proceed, measure memory with two real CLIs before the first release.',
    ].join('\n')
  }
  return [
    `**${lead} from ${p.name}** to "${short}"`,
    '',
    'The core should stay independent from the window toolkit, so a later move to Wails v3 only touches the adapter. ',
    'Persist every event first and emit it afterwards; a reconnecting UI then replays from the last sequence number instead of guessing. ',
    'For independent rounds, the only honest label is *soft isolation* until the canary test passes on this machine.',
    '',
    '- Keep answers short; details on request.',
    '- Decisions go into the shared context with their reasons.',
  ].join('\n')
}

function chunk(text: string): string[] {
  const parts: string[] = []
  const words = text.split(/(\s+)/)
  let current = ''
  for (const word of words) {
    current += word
    if (current.length >= 14) {
      parts.push(current)
      current = ''
    }
  }
  if (current) parts.push(current)
  return parts
}

class MockForum implements ForumApi {
  readonly mode = 'mock' as const

  private readonly workspace: Workspace
  private participants: Participant[]
  private topics: Topic[]
  private messages: Message[] = []
  private runs: Run[] = []
  private rounds: Round[] = []
  private revisions: ContextRevision[] = []
  private readonly log: Event[] = []
  private readonly listeners = new Set<(ev: Event) => void>()
  private readonly resyncListeners = new Set<() => void>()
  private readonly active = new Map<string, ActiveRun>()
  private readonly timers = new Set<ReturnType<typeof setTimeout>>()

  constructor() {
    this.workspace = { id: 'ws-1', name: 'Personal', rootDir: '~/projects', createdAt: nowIso(-86_400_000) }
    this.participants = [
      {
        id: 'p-claude',
        workspaceId: 'ws-1',
        name: 'Claude',
        provider: 'claude',
        model: 'sonnet',
        role: 'You review architecture and point out risks.',
        effort: 'high',
        maxWords: 150,
        mode: 'mentioned',
        enabled: true,
        profile: 'subscription',
        tools: 'none',
      },
      {
        id: 'p-codex',
        workspaceId: 'ws-1',
        name: 'Codex',
        provider: 'codex',
        model: 'gpt-5-codex',
        role: 'You check implementation details and CLI behaviour.',
        effort: 'medium',
        maxWords: 120,
        mode: 'mentioned',
        enabled: true,
        profile: 'subscription',
        tools: 'read_only',
      },
    ]
    this.topics = [
      {
        id: 't-stack',
        workspaceId: 'ws-1',
        title: 'Stack decision for the forum',
        kind: 'discussion',
        createdAt: nowIso(-7_200_000),
        updatedAt: nowIso(-600_000),
      },
      {
        id: 't-isolation',
        workspaceId: 'ws-1',
        title: 'How strict can independent rounds be?',
        kind: 'question',
        createdAt: nowIso(-3_600_000),
        updatedAt: nowIso(-3_000_000),
      },
    ]
    this.seedConversation()
  }

  private seedConversation(): void {
    const add = (topicId: string, authorKind: Message['authorKind'], body: string, participantId?: string, offset = 0) => {
      const m: Message = {
        id: nextId('m'),
        topicId,
        authorKind,
        participantId,
        body,
        mentions: [],
        visibility: 'visible',
        createdAt: nowIso(offset),
      }
      this.messages.push(m)
      return m
    }
    add('t-stack', 'human', 'Wails v2 or v3 for the first release? I want one toolchain and a stable window layer.', undefined, -7_100_000)
    add(
      't-stack',
      'agent',
      'Wails v2 is the stable branch and the core does not import it, so a later move to v3 only touches the window adapter. Take v2 now.',
      'p-claude',
      -7_000_000,
    )
    add(
      't-stack',
      'agent',
      'Agreed. `wails init -t react-ts` gives the official template; Angular would need manual `wails.json` wiring that nobody tests upstream.',
      'p-codex',
      -6_900_000,
    )
    add('t-isolation', 'human', 'Which leak channels remain once we pass the isolation flags to both CLIs?', undefined, -3_000_000)
    this.revisions.push({
      id: nextId('rev'),
      topicId: 't-stack',
      version: 1,
      status: 'accepted',
      authorKind: 'human',
      doc: {
        goal: 'Ship a local forum where a human, Claude Code and Codex discuss and decide.',
        constraints: ['macOS first', 'One maintainer', 'No credentials stored by the app'],
        decisions: [{ text: 'Wails v2 + Go + React/TypeScript', reason: 'stable branch, one toolchain' }],
        done: ['Domain model and orchestrator with tests'],
        current: 'Building the UI against the mock API',
        openQuestions: ['Can Codex exec restrict reads outside the run directory?'],
        references: [],
      },
      createdAt: nowIso(-6_000_000),
    })
  }

  // --- event log ----------------------------------------------------------

  private emit(type: string, topicId: string | undefined, payload: unknown): void {
    const ev: Event = { seq: this.log.length + 1, type, topicId, at: nowIso(), payload: clone(payload) }
    this.log.push(ev)
    for (const listener of this.listeners) {
      try {
        listener(clone(ev))
      } catch (err) {
        console.error('mock listener failed', err)
      }
    }
  }

  onEvent(cb: (ev: Event) => void): () => void {
    this.listeners.add(cb)
    return () => this.listeners.delete(cb)
  }

  onResync(cb: () => void): () => void {
    this.resyncListeners.add(cb)
    return () => this.resyncListeners.delete(cb)
  }

  async events(after: number, limit: number): Promise<Event[]> {
    const max = limit > 0 ? limit : 1000
    return clone(this.log.filter((ev) => ev.seq > after).slice(0, max))
  }

  // --- workspace, participants, topics ------------------------------------

  async capabilities(): Promise<Capabilities[]> {
    return clone(mockCapabilities)
  }

  async ensureWorkspace(): Promise<Workspace> {
    return clone(this.workspace)
  }

  async listWorkspaces(): Promise<Workspace[]> {
    return [clone(this.workspace)]
  }

  async createWorkspace(name: string): Promise<Workspace> {
    throw new Error(`the mock has a single workspace; cannot create "${name}"`)
  }

  async listParticipants(workspaceId: string): Promise<Participant[]> {
    return clone(this.participants.filter((p) => p.workspaceId === workspaceId))
  }

  async saveParticipant(participant: Participant): Promise<Participant> {
    const name = participant.name.trim()
    if (!name) throw new Error('participant name is required')
    const caps = mockCapabilities.find((c) => c.provider === participant.provider)
    if (!caps) throw new Error(`no adapter for provider ${participant.provider}`)
    if (participant.effort && !(caps.efforts ?? []).includes(participant.effort)) {
      throw new Error(`effort "${participant.effort}" is not offered by ${participant.provider}`)
    }
    const saved: Participant = {
      ...participant,
      name,
      id: participant.id || nextId('p'),
      enabled: participant.id ? participant.enabled : true,
      profile: participant.profile || 'subscription',
      tools: participant.tools || 'none',
      mode: participant.mode || 'mentioned',
    }
    const idx = this.participants.findIndex((p) => p.id === saved.id)
    if (idx >= 0) this.participants[idx] = saved
    else this.participants.push(saved)
    this.emit('participant.changed', undefined, { participant: saved })
    return clone(saved)
  }

  async listTopics(workspaceId: string): Promise<Topic[]> {
    return clone(
      this.topics
        .filter((t) => t.workspaceId === workspaceId)
        .sort((a, b) => (a.updatedAt < b.updatedAt ? 1 : a.updatedAt > b.updatedAt ? -1 : 0)),
    )
  }

  async createTopic(workspaceId: string, title: string, kind: TopicKind): Promise<Topic> {
    const trimmed = title.trim()
    if (!trimmed) throw new Error('topic title is required')
    const topic: Topic = {
      id: nextId('t'),
      workspaceId,
      title: trimmed,
      kind: kind || 'discussion',
      createdAt: nowIso(),
      updatedAt: nowIso(),
    }
    this.topics.push(topic)
    this.emit('topic.created', topic.id, { topic })
    return clone(topic)
  }

  private touchTopic(topicId: string): void {
    const topic = this.topics.find((t) => t.id === topicId)
    if (topic) topic.updatedAt = nowIso()
  }

  // --- thread ---------------------------------------------------------------

  async getThread(topicId: string, limit: number): Promise<Thread> {
    const topic = this.topics.find((t) => t.id === topicId)
    if (!topic) throw new Error(`topic ${topicId} not found`)
    const visible = this.messages.filter((m) => m.topicId === topicId && m.visibility === 'visible')
    const messages = limit > 0 ? visible.slice(-limit) : visible
    const current = this.currentContext(topicId)
    return clone({
      topic,
      messages,
      runs: this.runs.filter((r) => r.topicId === topicId),
      rounds: this.rounds.filter((r) => r.topicId === topicId),
      participants: this.participants,
      context: current,
      lastEventSeq: this.log.length,
    })
  }

  async runOutput(runId: string): Promise<string> {
    const run = this.runs.find((r) => r.id === runId)
    if (!run) throw new Error(`run ${runId} not found`)
    const round = run.roundId ? this.rounds.find((r) => r.id === run.roundId) : undefined
    if (round && round.kind === 'independent' && round.status !== 'revealed') {
      throw new Error('run output is sealed until the round is revealed')
    }
    const live = this.active.get(runId)
    if (live) return live.text
    const message = this.messages.find((m) => m.runId === runId)
    return message?.body ?? ''
  }

  // --- posting and rounds -----------------------------------------------

  async postMessage(input: PostInput): Promise<PostResult> {
    const body = input.body.trim()
    if (!body) throw new Error('message body is required')
    const topic = this.topics.find((t) => t.id === input.topicId)
    if (!topic) throw new Error(`topic ${input.topicId} not found`)
    const targets: string[] = []
    for (const id of input.mentions) {
      const p = this.participants.find((x) => x.id === id)
      if (!p) throw new Error(`unknown participant ${id}`)
      if (p.enabled && !targets.includes(id)) targets.push(id)
    }
    for (const p of this.participants) {
      if (p.mode === 'always' && p.enabled && !targets.includes(p.id)) targets.push(p.id)
    }
    const message: Message = {
      id: nextId('m'),
      topicId: input.topicId,
      parentId: input.parentId,
      authorKind: 'human',
      body,
      mentions: [...input.mentions],
      visibility: 'visible',
      createdAt: nowIso(),
    }
    this.messages.push(message)
    this.touchTopic(topic.id)
    this.emit('message.created', topic.id, { message })
    if (targets.length === 0) return clone({ message })

    const kind = kindFor(input.mode, targets.length)
    const round = this.startRound(topic.id, kind, body, targets, input.autoReveal, message.id)
    return clone({ message, round })
  }

  private startRound(
    topicId: string,
    kind: RoundKind,
    prompt: string,
    participantIds: string[],
    autoReveal: boolean,
    triggerMessageId: string,
  ): Round {
    const frozen = this.messages
      .filter((m) => m.topicId === topicId && m.visibility === 'visible' && m.id !== triggerMessageId)
      .map((m) => m.id)
    const round: Round = {
      id: nextId('round'),
      topicId,
      kind,
      status: 'running',
      prompt,
      participantIds,
      inputs: { contextRevisionId: this.currentContext(topicId)?.id, messageIds: frozen },
      autoReveal,
      createdAt: nowIso(),
    }
    this.rounds.push(round)
    this.emit('round.status_changed', topicId, { roundId: round.id, status: round.status })
    const first = kind === 'independent' ? participantIds : participantIds.slice(0, 1)
    for (const pid of first) this.createRun(round, pid, triggerMessageId)
    return round
  }

  private createRun(round: Round, participantId: string, triggerMessageId?: string): Run {
    const p = this.participants.find((x) => x.id === participantId)
    const run: Run = {
      id: nextId('run'),
      topicId: round.topicId,
      participantId,
      roundId: round.id,
      triggerMessageId,
      status: 'queued',
      profile: p?.profile ?? 'subscription',
      tools: p?.tools ?? 'none',
      isolation: round.kind === 'independent' ? 'soft' : 'none',
      usage: { inputTokens: 0, outputTokens: 0, cachedTokens: 0, costUsd: 0 },
      createdAt: nowIso(),
    }
    this.runs.push(run)
    this.emitRunStatus(run)
    const timer = setTimeout(() => {
      this.timers.delete(timer)
      this.beginRun(run.id)
    }, QUEUE_DELAY_MS)
    this.timers.add(timer)
    return run
  }

  private beginRun(runId: string): void {
    const run = this.runs.find((r) => r.id === runId)
    if (!run || run.status !== 'queued') return
    const round = this.rounds.find((r) => r.id === run.roundId)
    const participant = this.participants.find((p) => p.id === run.participantId)
    if (!round || !participant) {
      this.finishRun(run, 'failed', 'participant or round disappeared')
      return
    }
    run.status = 'running'
    run.startedAt = nowIso()
    this.emitRunStatus(run)
    const sealed = round.kind === 'independent'
    const state: ActiveRun = { timer: null, chunks: chunk(replyFor(participant, round.prompt, round.kind)), next: 0, text: '', outputIndex: 0 }
    this.active.set(run.id, state)
    if (participant.tools !== 'none') {
      this.emit('run.tool_activity', run.topicId, { runId: run.id, tool: 'Read', summary: 'prompt.md', status: 'completed' })
    }
    state.timer = setInterval(() => {
      if (state.next >= state.chunks.length) {
        this.finishRun(run, 'succeeded')
        return
      }
      const text = state.chunks[state.next]
      state.next += 1
      state.text += text
      this.emit('run.output', run.topicId, {
        runId: run.id,
        index: state.outputIndex,
        text: sealed ? '' : text,
        sealed,
      })
      state.outputIndex += 1
    }, CHUNK_INTERVAL_MS)
  }

  private finishRun(run: Run, status: RunStatus, error?: string): void {
    const live = this.active.get(run.id)
    if (live?.timer) clearInterval(live.timer)
    this.active.delete(run.id)
    const round = this.rounds.find((r) => r.id === run.roundId)
    const sealed = round?.kind === 'independent'
    const text = live?.text ?? ''
    if (text) {
      const message: Message = {
        id: nextId('m'),
        topicId: run.topicId,
        authorKind: 'agent',
        participantId: run.participantId,
        body: status === 'succeeded' ? text : `${text}\n\n_[incomplete: ${status}]_`,
        mentions: [],
        runId: run.id,
        roundId: run.roundId,
        visibility: sealed ? 'sealed' : 'visible',
        createdAt: nowIso(),
      }
      this.messages.push(message)
      run.resultMessageId = message.id
      this.emit('message.created', run.topicId, { message: sealed ? { ...message, body: '' } : message })
    }
    run.status = status
    run.error = error ?? (status === 'succeeded' ? undefined : `runner: ${status}`)
    run.usage = { inputTokens: 1200, outputTokens: Math.max(1, Math.round(text.length / 4)), cachedTokens: 800, costUsd: 0 }
    run.finishedAt = nowIso()
    this.emitRunStatus(run)
    this.touchTopic(run.topicId)
    if (round) this.settleRound(round)
  }

  private emitRunStatus(run: Run): void {
    this.emit('run.status_changed', run.topicId, {
      runId: run.id,
      participantId: run.participantId,
      roundId: run.roundId,
      status: run.status,
      error: run.error,
      usage: isTerminalRun(run.status) ? run.usage : undefined,
    })
  }

  private settleRound(round: Round): void {
    if (round.status !== 'running') return
    const runs = this.runs.filter((r) => r.roundId === round.id)
    if (runs.some((r) => !isTerminalRun(r.status))) return
    const succeeded = new Set(runs.filter((r) => r.status === 'succeeded').map((r) => r.participantId))
    const missing = round.participantIds.filter((pid) => !succeeded.has(pid))
    if (missing.length > 0) {
      const lastOK = runs.length > 0 && runs[runs.length - 1].status === 'succeeded'
      if (round.kind !== 'independent' && lastOK) {
        this.createRun(round, missing[0])
        return
      }
      this.setRoundStatus(round, 'partial')
      return
    }
    if (round.kind === 'independent' && !round.autoReveal) {
      this.setRoundStatus(round, 'awaiting_reveal')
      return
    }
    this.reveal(round)
  }

  private setRoundStatus(round: Round, status: RoundStatus): void {
    round.status = status
    if (status === 'revealed') round.revealedAt = nowIso()
    if (status !== 'running' && status !== 'awaiting_reveal') round.finishedAt = nowIso()
    this.emit('round.status_changed', round.topicId, { roundId: round.id, status })
  }

  private reveal(round: Round): void {
    if (!['running', 'awaiting_reveal', 'partial'].includes(round.status)) {
      throw new Error(`round: illegal transition ${round.status} -> revealed`)
    }
    for (const m of this.messages) {
      if (m.roundId === round.id && m.visibility === 'sealed') {
        m.visibility = 'visible'
        this.emit('message.revealed', round.topicId, { message: m })
      }
    }
    this.setRoundStatus(round, 'revealed')
  }

  async cancelRun(runId: string): Promise<void> {
    const run = this.runs.find((r) => r.id === runId)
    if (!run) throw new Error(`run ${runId} not found`)
    if (run.status === 'running') {
      this.finishRun(run, 'cancelled', 'runner: cancelled')
      return
    }
    if (run.status === 'queued') {
      run.status = 'cancelled'
      run.error = 'runner: cancelled'
      run.finishedAt = nowIso()
      this.emitRunStatus(run)
      const round = this.rounds.find((r) => r.id === run.roundId)
      if (round) this.settleRound(round)
    }
  }

  async cancelRound(roundId: string): Promise<void> {
    const round = this.rounds.find((r) => r.id === roundId)
    if (!round) throw new Error(`round ${roundId} not found`)
    if (['revealed', 'cancelled', 'interrupted'].includes(round.status)) throw new Error(`round is ${round.status}`)
    this.setRoundStatus(round, 'cancelled')
    for (const run of this.runs.filter((r) => r.roundId === roundId)) {
      if (run.status === 'queued') {
        run.status = 'cancelled'
        run.error = 'runner: cancelled'
        run.finishedAt = nowIso()
        this.emitRunStatus(run)
      } else if (run.status === 'running') {
        this.finishRun(run, 'cancelled', 'runner: cancelled')
      }
    }
  }

  async revealRound(roundId: string): Promise<void> {
    const round = this.rounds.find((r) => r.id === roundId)
    if (!round) throw new Error(`round ${roundId} not found`)
    if (round.status !== 'awaiting_reveal' && round.status !== 'partial') {
      throw new Error(`round: illegal transition ${round.status} -> revealed`)
    }
    this.reveal(round)
  }

  async retryRound(roundId: string): Promise<void> {
    const round = this.rounds.find((r) => r.id === roundId)
    if (!round) throw new Error(`round ${roundId} not found`)
    if (round.status !== 'partial') throw new Error(`round: illegal transition ${round.status} -> running`)
    const succeeded = new Set(
      this.runs.filter((r) => r.roundId === roundId && r.status === 'succeeded').map((r) => r.participantId),
    )
    this.setRoundStatus(round, 'running')
    for (const pid of round.participantIds) {
      if (succeeded.has(pid)) continue
      this.createRun(round, pid)
      if (round.kind !== 'independent') break
    }
  }

  // --- shared context ---------------------------------------------------

  private currentContext(topicId: string): ContextRevision | undefined {
    return this.revisions
      .filter((r) => r.topicId === topicId && r.status === 'accepted')
      .sort((a, b) => b.version - a.version)[0]
  }

  async saveContext(topicId: string, doc: ContextDoc): Promise<ContextRevision> {
    if (!this.topics.some((t) => t.id === topicId)) throw new Error(`topic ${topicId} not found`)
    const version = this.revisions.filter((r) => r.topicId === topicId).reduce((max, r) => Math.max(max, r.version), 0) + 1
    const rev: ContextRevision = {
      id: nextId('rev'),
      topicId,
      version,
      status: 'accepted',
      authorKind: 'human',
      doc: clone(doc),
      createdAt: nowIso(),
    }
    this.revisions.push(rev)
    this.emit('context.revised', topicId, { revision: rev })
    return clone(rev)
  }

  async listContextRevisions(topicId: string): Promise<ContextRevision[]> {
    return clone(this.revisions.filter((r) => r.topicId === topicId).sort((a, b) => a.version - b.version))
  }

  async setContextRevisionStatus(revisionId: string, status: RevisionStatus): Promise<void> {
    const rev = this.revisions.find((r) => r.id === revisionId)
    if (!rev) throw new Error(`revision ${revisionId} not found`)
    rev.status = status
    this.emit('context.revised', rev.topicId, { revision: rev })
  }

  /** Stops timers; used by tests and hot reloads. */
  dispose(): void {
    for (const t of this.timers) clearTimeout(t)
    for (const a of this.active.values()) if (a.timer) clearInterval(a.timer)
    this.active.clear()
  }
}

function kindFor(mode: AskMode, targets: number): RoundKind {
  if (mode === 'independent') return 'independent'
  if (mode === 'joint' || targets > 1) return 'joint'
  return 'single'
}

/** Creates a fresh mock backend with seeded data. */
export function createMockApi(): ForumApi {
  return new MockForum()
}
