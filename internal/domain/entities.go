package domain

import "time"

// Workspace groups topics and participants that belong to one project.
type Workspace struct {
	ID        ID        `json:"id"`
	Name      string    `json:"name"`
	RootDir   string    `json:"rootDir"`
	CreatedAt time.Time `json:"createdAt"`
}

// TopicKind says what a topic is used for. Issues and tasks are topics with a
// different kind, not a separate tracker.
type TopicKind string

const (
	TopicDiscussion TopicKind = "discussion"
	TopicQuestion   TopicKind = "question"
	TopicTask       TopicKind = "task"
)

// Topic is a thread of messages.
type Topic struct {
	ID          ID        `json:"id"`
	WorkspaceID ID        `json:"workspaceId"`
	Title       string    `json:"title"`
	Kind        TopicKind `json:"kind"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Provider identifies the CLI adapter behind a participant.
type Provider string

const (
	ProviderHuman  Provider = "human"
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
	ProviderFake   Provider = "fake"
)

// ParticipationMode controls when an agent participant is invoked.
type ParticipationMode string

const (
	// ParticipateWhenMentioned invokes the participant only when a message
	// mentions it. This is the default.
	ParticipateWhenMentioned ParticipationMode = "mentioned"
	// ParticipateAlways invokes the participant on every human message.
	ParticipateAlways ParticipationMode = "always"
	// ParticipateManual never invokes the participant automatically.
	ParticipateManual ParticipationMode = "manual"
)

// Participant is a configured agent or the human author.
type Participant struct {
	ID          ID       `json:"id"`
	WorkspaceID ID       `json:"workspaceId"`
	Name        string   `json:"name"`
	Provider    Provider `json:"provider"`
	Model       string   `json:"model,omitempty"`
	// Role is free-form instructions prepended to every prompt.
	Role string `json:"role,omitempty"`
	// Effort is a provider-specific effort level. Allowed values come from the
	// adapter's capabilities, not from a shared list.
	Effort string `json:"effort,omitempty"`
	// MaxWords is a soft limit communicated in the prompt and shown as a
	// metric; it is not enforced.
	MaxWords int               `json:"maxWords,omitempty"`
	Mode     ParticipationMode `json:"mode"`
	Enabled  bool              `json:"enabled"`
	// Profile selects how the adapter authenticates and which CLI flags it
	// uses. See RunProfile.
	Profile RunProfile `json:"profile"`
	// Tools is the default tool policy for this participant.
	Tools ToolPolicy `json:"tools"`
}

// RunProfile selects the authentication and startup mode of a CLI run.
type RunProfile string

const (
	// ProfileSubscription uses the login already stored by the installed CLI.
	// The CLI is started without its "bare" mode, so the adapter has to pass
	// explicit flags to keep hooks, MCP servers and memory out of the run.
	ProfileSubscription RunProfile = "subscription"
	// ProfileAPIKey starts the CLI in its minimal mode with an API key from
	// the environment. Nothing from the user's CLI configuration is loaded.
	ProfileAPIKey RunProfile = "api_key"
)

// ToolPolicy says which tools a run may use.
type ToolPolicy string

const (
	// ToolsNone disables every tool: the run is a plain discussion.
	ToolsNone ToolPolicy = "none"
	// ToolsReadOnly allows reading files inside the run directory.
	ToolsReadOnly ToolPolicy = "read_only"
	// ToolsWorkspaceWrite allows editing files inside the run directory and
	// running commands there, subject to the CLI's own sandbox.
	ToolsWorkspaceWrite ToolPolicy = "workspace_write"
)

// IsolationLevel describes how strongly a run is separated from the host and
// from other runs. It is a label the orchestrator derives from what the
// adapter can actually enforce, never a promise typed into a prompt.
type IsolationLevel string

const (
	// IsolationNone: the run sees whatever the CLI would see in a terminal.
	IsolationNone IsolationLevel = "none"
	// IsolationSoft: fresh session, private run directory with frozen inputs,
	// MCP servers and memory disabled, results read only from stdout. File
	// reads outside the run directory are discouraged but not proven
	// impossible.
	IsolationSoft IsolationLevel = "soft"
	// IsolationStrict: IsolationSoft plus an OS or CLI sandbox that has been
	// verified on this machine to block reads outside the run directory.
	IsolationStrict IsolationLevel = "strict"
)

// AuthorKind says who wrote a message.
type AuthorKind string

const (
	AuthorHuman  AuthorKind = "human"
	AuthorAgent  AuthorKind = "agent"
	AuthorSystem AuthorKind = "system"
)

// Visibility says whether a message can be shown and used as context.
type Visibility string

const (
	// VisibilityVisible messages are part of the topic.
	VisibilityVisible Visibility = "visible"
	// VisibilitySealed messages belong to an unfinished independent round and
	// must not be shown or given to other participants until the round is
	// revealed.
	VisibilitySealed Visibility = "sealed"
)

// Message is one post in a topic. Messages are immutable once created; a
// correction is a new message that references the original.
type Message struct {
	ID            ID         `json:"id"`
	TopicID       ID         `json:"topicId"`
	ParentID      ID         `json:"parentId,omitempty"`
	AuthorKind    AuthorKind `json:"authorKind"`
	ParticipantID ID         `json:"participantId,omitempty"`
	Body          string     `json:"body"`
	// Mentions lists the participants addressed by the message.
	Mentions   []ID       `json:"mentions,omitempty"`
	RunID      ID         `json:"runId,omitempty"`
	RoundID    ID         `json:"roundId,omitempty"`
	Visibility Visibility `json:"visibility"`
	// CorrectsID points at the message this one corrects, with the reason in
	// the body. The original stays unchanged.
	CorrectsID ID        `json:"correctsId,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Usage is the token and cost accounting reported by a CLI for one run. Cost
// is the CLI's own estimate and may be zero for subscription logins.
type Usage struct {
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CachedTokens int64   `json:"cachedTokens"`
	CostUSD      float64 `json:"costUsd"`
}

// Run is one invocation of a participant's CLI.
type Run struct {
	ID            ID `json:"id"`
	TopicID       ID `json:"topicId"`
	ParticipantID ID `json:"participantId"`
	RoundID       ID `json:"roundId,omitempty"`
	// TriggerMessageID is the message that caused this run.
	TriggerMessageID ID             `json:"triggerMessageId,omitempty"`
	Status           RunStatus      `json:"status"`
	Profile          RunProfile     `json:"profile"`
	Tools            ToolPolicy     `json:"tools"`
	Isolation        IsolationLevel `json:"isolation"`
	// WorkDir is the private directory the CLI was started in.
	WorkDir string `json:"workDir,omitempty"`
	// NativeSessionID is the CLI's own session or thread identifier. It is
	// an adapter detail kept for diagnostics and optional resume; the forum's
	// history lives in Message, not in the CLI's files.
	NativeSessionID string    `json:"nativeSessionId,omitempty"`
	ResultMessageID ID        `json:"resultMessageId,omitempty"`
	Error           string    `json:"error,omitempty"`
	Usage           Usage     `json:"usage"`
	CreatedAt       time.Time `json:"createdAt"`
	StartedAt       time.Time `json:"startedAt,omitempty"`
	FinishedAt      time.Time `json:"finishedAt,omitempty"`
}

// RoundKind says how the runs of a round relate to each other.
type RoundKind string

const (
	// RoundSingle: one participant answers.
	RoundSingle RoundKind = "single"
	// RoundJoint: participants answer in sequence and see each other's
	// answers.
	RoundJoint RoundKind = "joint"
	// RoundIndependent: participants answer in parallel without seeing each
	// other; results are revealed together once all runs succeed.
	RoundIndependent RoundKind = "independent"
)

// FrozenInputs is the snapshot of material every run in a round receives.
// It is fixed when the round starts; later messages wait for the next round.
type FrozenInputs struct {
	ContextRevisionID ID   `json:"contextRevisionId,omitempty"`
	MessageIDs        []ID `json:"messageIds"`
	ArtifactIDs       []ID `json:"artifactIds,omitempty"`
}

// Round is a coordinated set of runs.
type Round struct {
	ID             ID           `json:"id"`
	TopicID        ID           `json:"topicId"`
	Kind           RoundKind    `json:"kind"`
	Status         RoundStatus  `json:"status"`
	Prompt         string       `json:"prompt"`
	ParticipantIDs []ID         `json:"participantIds"`
	Inputs         FrozenInputs `json:"inputs"`
	// AutoReveal reveals an independent round as soon as every run succeeds.
	AutoReveal bool      `json:"autoReveal"`
	CreatedAt  time.Time `json:"createdAt"`
	RevealedAt time.Time `json:"revealedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

// RevisionStatus says whether a context revision is in force.
type RevisionStatus string

const (
	// RevisionAccepted revisions are the current shared context when they are
	// the newest accepted revision of the topic.
	RevisionAccepted RevisionStatus = "accepted"
	// RevisionProposed revisions were written by an agent and wait for the
	// human to accept or discard them.
	RevisionProposed RevisionStatus = "proposed"
	// RevisionDiscarded revisions are kept for history only.
	RevisionDiscarded RevisionStatus = "discarded"
)

// ContextRevision is one version of a topic's shared context.
type ContextRevision struct {
	ID            ID             `json:"id"`
	TopicID       ID             `json:"topicId"`
	Version       int            `json:"version"`
	Status        RevisionStatus `json:"status"`
	AuthorKind    AuthorKind     `json:"authorKind"`
	ParticipantID ID             `json:"participantId,omitempty"`
	RunID         ID             `json:"runId,omitempty"`
	Doc           ContextDoc     `json:"doc"`
	CreatedAt     time.Time      `json:"createdAt"`
}

// ArtifactVersion is one version of a file attached to a topic. The bytes
// live in the workspace's managed directory; the row records provenance.
type ArtifactVersion struct {
	ID        ID        `json:"id"`
	TopicID   ID        `json:"topicId"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	Path      string    `json:"path"`
	RunID     ID        `json:"runId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
