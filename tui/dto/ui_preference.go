package dto

type UIPreference struct {
	Version      int    `json:"version"`
	Theme        string `json:"theme"`
	Icons        string `json:"icons"`
	ReduceMotion bool   `json:"reduce_motion"`
}
