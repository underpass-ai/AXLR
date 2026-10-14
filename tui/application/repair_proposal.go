package application

type RepairProposal struct {
	Repository, Base, Branch string
	Title, Body, Trailer     string
	// Number is the open pull request on a later round, zero on the first.
	Number int
}
