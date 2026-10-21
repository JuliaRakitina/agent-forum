import type { AskMode, RoundKind, RoundStatus, RunStatus } from '../api/types'

export const ASK_MODES: { value: AskMode; label: string; hint: string }[] = [
  { value: 'one', label: 'Ask one', hint: 'One participant answers; several mentions become a joint discussion.' },
  { value: 'joint', label: 'Discuss jointly', hint: 'Participants answer in turn and see earlier answers.' },
  { value: 'independent', label: 'Independent answers', hint: 'Participants answer in parallel; results stay sealed until all succeed.' },
]

export function roundKindLabel(kind: RoundKind): string {
  switch (kind) {
    case 'single':
      return 'Ask one'
    case 'joint':
      return 'Joint discussion'
    case 'independent':
      return 'Independent round'
  }
}

export function roundStatusLabel(status: RoundStatus): string {
  switch (status) {
    case 'running':
      return 'in progress'
    case 'awaiting_reveal':
      return 'sealed, ready to reveal'
    case 'revealed':
      return 'revealed'
    case 'partial':
      return 'partial: a run did not succeed'
    case 'cancelled':
      return 'cancelled'
    case 'interrupted':
      return 'interrupted by an application restart'
  }
}

export function runStatusLabel(status: RunStatus): string {
  switch (status) {
    case 'queued':
      return 'queued'
    case 'running':
      return 'working'
    case 'succeeded':
      return 'done'
    case 'failed':
      return 'failed'
    case 'cancelled':
      return 'cancelled'
    case 'timed_out':
      return 'timed out'
    case 'interrupted':
      return 'interrupted'
  }
}
