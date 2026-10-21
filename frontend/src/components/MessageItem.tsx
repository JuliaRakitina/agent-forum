import type { Message, Participant } from '../api/types'
import { authorLabel, initials } from '../lib/participants'
import { formatTime } from '../lib/time'
import { Markdown } from './Markdown'

interface Props {
  message: Message
  participants: Participant[]
}

export function MessageItem({ message, participants }: Props) {
  const author = authorLabel(message, participants)
  const sealed = message.visibility === 'sealed'
  return (
    <article className={`message message-${message.authorKind}`} data-message-id={message.id}>
      <div className="avatar" aria-hidden="true">
        {initials(author)}
      </div>
      <div className="message-body">
        <header className="message-header">
          <span className="message-author">{author}</span>
          <time className="message-time" dateTime={message.createdAt}>
            {formatTime(message.createdAt)}
          </time>
          {message.correctsId && <span className="badge">correction</span>}
        </header>
        {sealed ? <p className="sealed">Sealed until the round is revealed.</p> : <Markdown text={message.body} />}
      </div>
    </article>
  )
}
