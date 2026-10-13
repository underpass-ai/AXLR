package application

type memoryLinkArgument struct {
	Ref      string `json:"ref"`
	Rel      string `json:"rel"`
	Why      string `json:"why"`
	Evidence string `json:"evidence"`
}
