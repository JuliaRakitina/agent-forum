import type { Participant, Round, Run } from '../api/types'
import { roundKindLabel, roundStatusLabel } from '../lib/labels'
import { participantName } from '../lib/participants'
import type { StreamBuffer } from '../state/threadReducer'
import { RunCard } from './RunCard'

interface Props {
  round: Round
  runs: Run[]
  participants: Participant[]
  streams: Record<string, StreamBuffer>
  tools: Record<string, string>
  onCancelRun: (runId: string) => void
  onCancelRound: (roundId: string) => void
  onReveal: (roundId: string) => void
  onRetry: (roundId: string) => void
}

export function RoundBanner({ round, runs, participants, streams, tools, onCancelRun, onCancelRound, onReveal, onRetry }: Props) {
  const names = round.participantIds.map((id) => participantName(participants, id)).join(', ')
  const canReveal = round.status === 'awaiting_reveal' || round.status === 'partial'
  const canRetry = round.status === 'partial'
  const canCancel = round.status === 'running'
  return (
    <section className={`round round-${round.status}`} data-round-id={round.id} aria-label={`${roundKindLabel(round.kind)} ${roundStatusLabel(round.status)}`}>
      <header className="round-header">
        <span className="round-kind">{roundKindLabel(round.kind)}</span>
        <span className={`chip chip-round-${round.status}`}>{roundStatusLabel(round.status)}</span>
        <span className="muted round-names">{names}</span>
        <span className="round-actions">
          {canReveal && (
            <button type="button" className="btn btn-small btn-primary" onClick={() => onReveal(round.id)}>
              Reveal
            </button>
          )}
          {canRetry && (
            <button type="button" className="btn btn-small" onClick={() => onRetry(round.id)}>
              Retry
            </button>
          )}
          {canCancel && (
            <button type="button" className="btn btn-small btn-ghost" onClick={() => onCancelRound(round.id)}>
              Cancel round
            </button>
          )}
        </span>
      </header>
      {round.kind === 'independent' && round.status === 'awaiting_reveal' && (
        <p className="round-note">All answers are in. They were written without seeing each other; reveal them together.</p>
      )}
      {round.status === 'partial' && (
        <p className="round-note">Nothing was revealed automatically. Retry the failed participants, reveal what exists, or cancel.</p>
      )}
      {runs.length > 0 && (
        <div className="round-runs">
          {runs.map((run) => (
            <RunCard key={run.id} run={run} participants={participants} stream={streams[run.id]} toolLine={tools[run.id]} onCancel={onCancelRun} />
          ))}
        </div>
      )}
    </section>
  )
}
