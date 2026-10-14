package application

// TokenCounterPort counts the tokens of a text.
type TokenCounterPort interface {
	Count(text string) int
}
