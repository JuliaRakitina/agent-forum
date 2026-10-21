import type { Participant, Run } from '../api/types'
import { isTerminalRun } from '../api/types'
import { runStatusLabel } from '../lib/labels'
import { participantName } from '../lib/participants'
import type { StreamBuffer } from '../state/threadReducer'

interface Props {
  run: Run
  participants: Participant[]
  stream: StreamBuffer | undefined
  toolLine: string | undefined
  onCancel: (runId: string) => void
}

export function RunCard({ run, participants, stream, toolLine, onCancel }: Props) {
  const terminal = isTerminalRun(run.status)
  const sealed = stream?.sealed ?? false
  const text = stream?.text ?? ''
  return (
    <div className={`run-card run-${run.status}`} data-run-id={run.id}>
      <header className="run-header">
        <span className="run-name">{participantName(participants, run.participantId)}</span>
        <span className={`chip chip-${run.status}`}>{runStatusLabel(run.status)}</span>
        {run.isolation === 'soft' && <span className="chip chip-muted">soft isolation</span>}
        {!terminal && (
          <button type="button" className="btn btn-small btn-ghost" onClick={() => onCancel(run.id)}>
            Cancel
          </button>
        )}
      </header>
      {toolLine && <p className="run-tool">{toolLine}</p>}
      {!terminal && sealed && (
        <p className="run-working">
          working… {stream && stream.chunks > 0 ? `${stream.chunks} chunks received, hidden until the reveal` : ''}
        </p>
      )}
      {!terminal && !sealed && (text ? <pre className="run-stream">{text}</pre> : <p className="run-working">working…</p>)}
      {terminal && run.error && <p className="run-error">{run.error}</p>}
      {terminal && !run.error && <p className="run-working">finished without text</p>}
    </div>
  )
}
