import type { ApiMode } from '../api/types'

interface Props {
  workspaceName: string
  topicTitle: string | null
  mode: ApiMode
}

export function TopBar({ workspaceName, topicTitle, mode }: Props) {
  return (
    <header className="topbar">
      <div className="topbar-brand">
        <span className="topbar-app">Agent Forum</span>
        <span className="topbar-sep" aria-hidden="true">
          /
        </span>
        <span className="topbar-workspace">{workspaceName}</span>
      </div>
      <h1 className="topbar-title">{topicTitle ?? 'No topic selected'}</h1>
      <div className="topbar-status" title={mode === 'wails' ? 'Connected to the Go core' : 'In-memory mock backend'}>
        <span className={`dot ${mode === 'wails' ? 'dot-on' : 'dot-mock'}`} aria-hidden="true" />
        {mode === 'wails' ? 'Wails' : 'Mock'}
      </div>
    </header>
  )
}
