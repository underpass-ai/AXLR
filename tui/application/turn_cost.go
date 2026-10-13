package application

// TurnCost is the turn's share of the session's usage ledger.
type TurnCost struct {
	Requests int     `json:"requests"`
	USD      float64 `json:"usd"`
}
