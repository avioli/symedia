package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

type CommandFunc func([]string) error
type Command struct {
	Name        string
	UsageName   string
	Description string
	Callback    CommandFunc
}
type Commands []*Command

var registeredCommands Commands

func (cmds Commands) String() string {
	var buffer bytes.Buffer
	var names []string
	lines := make(map[string]string)
	maxLen := 0

	for _, command := range cmds {
		n := command.UsageName
		if n == "" {
			n = command.Name
		}
		names = append(names, n)
		lines[n] = command.Description
		if len(n) > maxLen {
			maxLen = len(n)
		}
	}
	sort.Strings(names)

	for _, n := range names {
		buffer.WriteString(fmt.Sprintf("  %s  %s\n", n+strings.Repeat(" ", maxLen-len(n)), lines[n]))
	}

	return buffer.String()
}

func registerCommand(name string, desc string, callback CommandFunc) (*Command, error) {
	cmd := Command{
		Name:        name,
		UsageName:   "",
		Description: desc,
		Callback:    callback,
	}
	registeredCommands = append(registeredCommands, &cmd)
	return &cmd, nil
}
