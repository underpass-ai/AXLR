package domain

type Fault struct {
	Status  string
	Code    string
	Message string
}

func (f *Fault) Error() string { return f.Message }
func Reject(code, message string) error {
	return &Fault{Status: "rejected", Code: code, Message: message}
}
func Fail(code, message string) error { return &Fault{Status: "failed", Code: code, Message: message} }
