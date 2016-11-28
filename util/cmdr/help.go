package cmdr

import (
	"bytes"
	"flag"
)

// Usage prints the usage message.
//func (h *Help) Usage(name string) string {
//	return fmt.Sprintf("usage: %s %s", name, h.Args)
//}

// Help is similar to PrintHelp, except it returns the result as a string
// instead of writing to the CLI's default Output.
func (cli *CLI) Help(base Command, name string, args []string) (string, error) {
	b := &bytes.Buffer{}
	err := cli.printHelp(NewOutput(b), base, name, args)
	return b.String(), err
}

func (cli *CLI) printHelp(out *Output, base Command, name string, args []string) error {
	if len(args) == 0 {
		help := base.Help()
		out.Println(help)
		cli.printSubcommands(out, base, name)
		cli.printOptions(out, base, name)
		return nil
	}
	hasSubCommands, ok := base.(Subcommander)
	if !ok {
		return UsageErrorf("%q does not have any subcommands", name)
	}
	scs := hasSubCommands.Subcommands()
	subcommandName := args[0]
	name = name + " " + subcommandName
	sc, ok := scs[subcommandName]
	if !ok {
		return UsageErrorf("command %q does not exist", name)
	}
	args = args[1:]
	return cli.printHelp(out, sc, name, args)
}

func (cli *CLI) printSubcommands(out *Output, c Command, name string) {
	subcommander, ok := c.(Subcommander)
	if !ok {
		return
	}
	cs := subcommander.Subcommands()
	out.Println("\nsubcommands:")
	out.Indent()
	defer out.Outdent()
	out.Table(commandTable(cs))
}

func (cli *CLI) printOptions(out *Output, command Command, name string) {
	addsFlags, ok := command.(AddsFlags)
	if !ok {
		return
	}
	out.Println("\noptions:")
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	addsFlags.AddFlags(fs)
	fs.SetOutput(out)
	fs.PrintDefaults()
}

func commandTable(cs Commands) [][]string {
	t := make([][]string, len(cs))
	for i, name := range cs.SortedKeys() {
		t[i] = make([]string, 2)
		t[i][0] = name
	}
	return t
}
