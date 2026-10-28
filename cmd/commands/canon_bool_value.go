package commands

import "github.com/spf13/pflag"

// boolValue is a stand-in bool flag value for the implicit help flags.
type boolValue struct{}

func (boolValue) String() string { return "false" }
func (boolValue) Set(string) error { return nil }
func (boolValue) Type() string { return "bool" }

var _ pflag.Value = boolValue{}
