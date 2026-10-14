package storage

type sessionSyncVerdict struct {
	Wave    int    `json:"wave"`
	Verdict string `json:"verdict"`
}
