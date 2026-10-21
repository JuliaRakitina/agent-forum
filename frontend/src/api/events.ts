// Event payload parsing. Payload shapes mirror internal/domain/events.go.
import type {
  ContextRevision,
  Event,
  Message,
  Participant,
  RoundStatus,
  RunStatus,
  Topic,
  Usage,
} from './types'

export interface RunStatusPayload {
  runId: string
  participantId: string
  roundId?: string
  status: RunStatus
  error?: string
  usage?: Usage
}

export interface RunOutputPayload {
  runId: string
  index: number
  text: string
  sealed: boolean
}

export interface RunToolActivityPayload {
  runId: string
  tool: string
  summary: string
  status: string
}

export interface RoundStatusPayload {
  roundId: string
  status: RoundStatus
}

export interface MessagePayload {
  message: Message
}

export interface ContextRevisedPayload {
  revision: ContextRevision
}

export interface ParticipantPayload {
  participant: Participant
}

export interface TopicPayload {
  topic: Topic
}

export type ParsedEvent =
  | { type: 'topic.created'; payload: TopicPayload }
  | { type: 'message.created' | 'message.revealed'; payload: MessagePayload }
  | { type: 'run.status_changed'; payload: RunStatusPayload }
  | { type: 'run.output'; payload: RunOutputPayload }
  | { type: 'run.tool_activity'; payload: RunToolActivityPayload }
  | { type: 'round.status_changed'; payload: RoundStatusPayload }
  | { type: 'context.revised'; payload: ContextRevisedPayload }
  | { type: 'participant.changed'; payload: ParticipantPayload }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function hasString(record: Record<string, unknown>, key: string): boolean {
  return typeof record[key] === 'string'
}

/**
 * Narrows an event to a typed payload. Unknown event types and malformed
 * payloads yield null so a newer backend never breaks the UI.
 */
export function parseEvent(ev: Event): ParsedEvent | null {
  const p = ev.payload
  if (!isRecord(p)) return null
  switch (ev.type) {
    case 'topic.created':
      return isRecord(p.topic) && hasString(p.topic, 'id') ? { type: ev.type, payload: p as unknown as TopicPayload } : null
    case 'message.created':
    case 'message.revealed':
      return isRecord(p.message) && hasString(p.message, 'id')
        ? { type: ev.type, payload: p as unknown as MessagePayload }
        : null
    case 'run.status_changed':
      return hasString(p, 'runId') && hasString(p, 'status')
        ? { type: ev.type, payload: p as unknown as RunStatusPayload }
        : null
    case 'run.output':
      return hasString(p, 'runId') && typeof p.index === 'number'
        ? { type: ev.type, payload: p as unknown as RunOutputPayload }
        : null
    case 'run.tool_activity':
      return hasString(p, 'runId') ? { type: ev.type, payload: p as unknown as RunToolActivityPayload } : null
    case 'round.status_changed':
      return hasString(p, 'roundId') && hasString(p, 'status')
        ? { type: ev.type, payload: p as unknown as RoundStatusPayload }
        : null
    case 'context.revised':
      return isRecord(p.revision) && hasString(p.revision, 'id')
        ? { type: ev.type, payload: p as unknown as ContextRevisedPayload }
        : null
    case 'participant.changed':
      return isRecord(p.participant) && hasString(p.participant, 'id')
        ? { type: ev.type, payload: p as unknown as ParticipantPayload }
        : null
    default:
      return null
  }
}
