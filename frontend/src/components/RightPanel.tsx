import { useState } from 'react'
import type { Capabilities, ContextDoc, ContextRevision, Participant, Workspace } from '../api/types'
import { ContextPanel } from './ContextPanel'
import { ParticipantsPanel } from './ParticipantsPanel'

interface Props {
  workspace: Workspace
  topicId: string | null
  context: ContextRevision | null
  participants: Participant[]
  capabilities: Capabilities[]
  onSaveContext: (doc: ContextDoc) => Promise<unknown>
  onSaveParticipant: (p: Participant) => Promise<unknown>
}

type Tab = 'context' | 'participants'

export function RightPanel({ workspace, topicId, context, participants, capabilities, onSaveContext, onSaveParticipant }: Props) {
  const [tab, setTab] = useState<Tab>('context')
  return (
    <aside className="right-panel">
      <div className="tabs" role="tablist">
        <button type="button" role="tab" aria-selected={tab === 'context'} className={tab === 'context' ? 'is-on' : ''} onClick={() => setTab('context')}>
          Context
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'participants'}
          className={tab === 'participants' ? 'is-on' : ''}
          onClick={() => setTab('participants')}
        >
          Participants
        </button>
      </div>
      <div className="panel-body" role="tabpanel">
        {tab === 'context' ? (
          <ContextPanel topicId={topicId} context={context} onSave={onSaveContext} />
        ) : (
          <ParticipantsPanel workspace={workspace} participants={participants} capabilities={capabilities} onSave={onSaveParticipant} />
        )}
      </div>
    </aside>
  )
}
