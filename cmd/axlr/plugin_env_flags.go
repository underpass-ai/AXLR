package main

import "fmt"

type pluginEnvFlags []string

func (p *pluginEnvFlags) String() string {
	return fmt.Sprintf("%d plugin environment entries", len(*p))
}
func (p *pluginEnvFlags) Set(value string) error {
	*p = append(*p, value)
	return nil
}
