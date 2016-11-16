package main

import (
	"fmt"
	"github.com/docopt/docopt-go"
	"os"
	"path"
)

var (
	Version string
	Build   string
	AppName string
)

func cmdHelp(argv []string) (err error) {
	usage := fmt.Sprint(`Usage: `, AppName, ` help [<command>] [<args>...]

Give help for given command
`)
	args, _ := docopt.Parse(usage, argv, true, "", false)

	if cmd, ok := args["<command>"].(string); ok {
		return runCommand(cmd, []string{"--help"})
	}

	return mainApp([]string{"--help"})
}

func runCommand(cmd string, args []string) (err error) {
	argv := make([]string, 1)
	argv[0] = cmd
	argv = append(argv, args...)

	if cmd == "help" {
		return cmdHelp(argv)
	}

	for _, command := range registeredCommands {
		if command.Name == cmd {
			return command.Callback(argv)
		}
	}

	return fmt.Errorf("%s is not a valid command. See '%s help'", cmd, AppName)
}

func mainApp(argv []string) (err error) {
	usage := fmt.Sprint(`Usage: `, AppName, ` <command> [<args>...]
       `, AppName, ` -h | --help | --version

Options:
  -h, --help  print this help, then exit
  --version   print version and build, then exit

Commands:
`, registeredCommands, `
See '`, AppName, ` help <command>' for more information on a specific command.
`)
	args, _ := docopt.Parse(usage, argv, true, "", true)

	if args["--version"].(bool) {
		fmt.Printf("Version: %s\nCommit: %s", Version, Build)
		os.Exit(0)
	}

	// fmt.Println("global arguments:")
	// fmt.Println(args)

	// fmt.Println("command arguments:")
	cmd := args["<command>"].(string)
	cmdArgs := args["<args>"].([]string)

	return runCommand(cmd, cmdArgs)
}

func init() {
	AppName = path.Base(os.Args[0])

	if cmd, err := registerCommand("help", "Print help for specific command.", cmdHelp); err == nil {
		cmd.UsageName = "help <command>"
	}
}

func main() {
	err := mainApp(nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
