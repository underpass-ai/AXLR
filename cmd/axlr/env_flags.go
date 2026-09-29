package main

import "fmt"

type envFlags []string

func (e *envFlags) String() string     { return fmt.Sprintf("%d entries", len(*e)) }
func (e *envFlags) Set(v string) error { *e = append(*e, v); return nil }
