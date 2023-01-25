package chord

//go:generate stringer -type=kvTargetType -linecomment

type kvTargetType int

const (
	targetLocal       kvTargetType = iota // Local
	targetRemote                          // Remote
	targetReplication                     // Replication
)
