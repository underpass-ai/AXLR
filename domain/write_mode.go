package domain

import "errors"

type WriteMode string

const (
	CreateMode  WriteMode = "create"
	ReplaceMode WriteMode = "replace"
)

func NewWriteMode(s string) (WriteMode, error) {
	mode := WriteMode(s)
	if mode != CreateMode && mode != ReplaceMode {
		return "", errors.New("mode must be create or replace")
	}
	return mode, nil
}
