package application

// ForgetReceipt says what was closed, its text, and the entry that
// supersedes it.
type ForgetReceipt struct {
	About, Ref, Text, Superseding string
}
