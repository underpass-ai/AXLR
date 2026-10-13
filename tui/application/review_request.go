package application

type ReviewRequest struct {
	// SessionModel is the reviewer's model unless reviewer_model is set.
	SessionModel string
	Rubric       string
	Draft        string
	// ReturnReason is the person's reason when the draft came back to review.
	ReturnReason string
}
