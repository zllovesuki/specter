package waf

import "time"

// Status reports this server's observations, separately from saved shared intent.
type Status struct {
	Configured          bool       `json:"configured"`
	Enabled             bool       `json:"enabled"`
	Ready               bool       `json:"ready"`
	ActiveVersion       string     `json:"active_version"`
	PreviousVersion     string     `json:"previous_version"`
	EngineVersion       string     `json:"engine_version"`
	Channel             string     `json:"channel"`
	PinnedVersion       string     `json:"pinned_version,omitempty"`
	AppliedVersion      string     `json:"applied_version"`
	LastCheck           *time.Time `json:"last_check,omitempty"`
	LastSuccessfulCheck *time.Time `json:"last_successful_check,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	Updating            bool       `json:"updating"`
}
