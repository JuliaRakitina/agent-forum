import type { Message, Participant } from '../api/types'

export function participantById(participants: Participant[], id: string | undefined): Participant | undefined {
  if (!id) return undefined
  return participants.find((p) => p.id === id)
}

export function participantName(participants: Participant[], id: string | undefined): string {
  return participantById(participants, id)?.name ?? 'Agent'
}

/** Resolves the display name of a message author. */
export function authorLabel(message: Message, participants: Participant[]): string {
  switch (message.authorKind) {
    case 'human':
      return 'You'
    case 'system':
      return 'System'
    default:
      return participantName(participants, message.participantId)
  }
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  return parts
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase() ?? '')
    .join('')
}

/** Agent participants the human can address. */
export function mentionable(participants: Participant[]): Participant[] {
  return participants.filter((p) => p.provider !== 'human')
}
