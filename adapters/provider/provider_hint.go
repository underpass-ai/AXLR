package provider

// ProviderHint names what the person can do about a provider error; the
// console words it in its language.
type ProviderHint string

const (
	HintNone ProviderHint = ""
	// HintChooseModel: the model id is not one the provider serves.
	HintChooseModel ProviderHint = "choose_model"
	// HintCheckKey: the provider refused the key.
	HintCheckKey ProviderHint = "check_key"
	// HintCredits: the account has no credits left.
	HintCredits ProviderHint = "credits"
	// HintRetryLater: the provider is rate limiting the requests.
	HintRetryLater ProviderHint = "retry_later"
)
