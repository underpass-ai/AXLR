package main

import "fmt"

type pluginFlags []string

func (p *pluginFlags) String() string { return fmt.Sprintf("%d manifests", len(*p)) }
func (p *pluginFlags) Set(value string) error {
	*p = append(*p, value)
	return nil
}
