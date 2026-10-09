package storage

import "fmt"

// maxSessionBudgetUSD bounds max_session_usd: a larger value is almost
// certainly a typo, and no limit is written as 0.
const maxSessionBudgetUSD = 10000

func (s UserSettings) validateSessionBudget() error {
	if !(s.MaxSessionUSD >= 0 && s.MaxSessionUSD <= maxSessionBudgetUSD) {
		return fmt.Errorf("settings max_session_usd must be between 0 and %d dollars", maxSessionBudgetUSD)
	}
	return nil
}
