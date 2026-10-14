package madesetup

type refusal struct{ Code, Message string }

func (r *refusal) Error() string { return r.Code + ": " + r.Message }
