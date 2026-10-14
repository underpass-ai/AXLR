package application

type ReviewVerdict struct {
	Accepted bool
	Findings []string
	// Model is the model that judged, for the record.
	Model string
}
