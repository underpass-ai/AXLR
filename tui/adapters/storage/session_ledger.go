package storage

type sessionLedger struct {
	Step         string `json:"step"`
	Iteration    int    `json:"iteration"`
	Text         string `json:"text"`
	FirstMessage int    `json:"first_message"`
	LastMessage  int    `json:"last_message"`
}
