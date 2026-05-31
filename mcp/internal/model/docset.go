package model

import "time"

// DocSet represents an indexed version of Spring Security documentation.
type DocSet struct {
	Ref        string
	CommitSha  string
	BuiltAt    time.Time
	ChunkCount int
	SourceType SourceType
}
