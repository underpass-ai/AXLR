package domain

import "errors"

type WorkingDirectory string

func NewWorkingDirectory(s string) (WorkingDirectory, error) {
	if s == "" || s == "." {
		return ".", nil
	}
	p, err := NewRelativePath(s)
	if err != nil {
		return "", errors.New("invalid working directory")
	}
	return WorkingDirectory(p), nil
}
