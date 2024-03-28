package timing

import "time"

const (
	ChordStabilizeInterval        = time.Second * 3
	ChordFixFingerInterval        = time.Second * 5
	ChordPredecessorCheckInterval = time.Second * 7
)
