package overlay

//go:generate stringer -type=direction -linecomment

type direction int

const (
	directionIncoming direction = iota // Incoming
	directionOutgoing                  // Outgoing
)
