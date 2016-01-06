package commands

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/opentable/sous/core"
	"github.com/opentable/sous/deploy"
	"github.com/opentable/sous/tools"
	"github.com/opentable/sous/tools/cli"
	"github.com/opentable/sous/tools/cmd"
	"github.com/opentable/sous/tools/docker"
	"github.com/opentable/sous/tools/yaml"
)

var contractsFlags = flag.NewFlagSet("contracts", flag.ExitOnError)

var timeoutFlag = contractsFlags.Duration("timeout", 10*time.Second, "per-contract timeout")
var dockerImage = contractsFlags.String("image", "", "run contracts against a pre-built Docker image")

func ContractsHelp() string {
	return `sous contracts tests your project conforms to necessary contracts to run successfully on the OpenTable Mesos platform.`
}

func Contracts(sous *core.Sous, args []string) {
	contractsFlags.Parse(args)
	args = contractsFlags.Args()
	if len(args) != 1 {
		cli.Fatalf("You must supply one argument: the docker image you want to run contracts against.")
	}

	image := args[0]
	if !docker.ImageExists(image) {
		cli.Logf("Image %q not found locally; pulling...")
		docker.Pull(image)
	}

	state, err := deploy.Parse(".")
	if err != nil {
		cli.Fatalf("Unable to parse state: %s", err)
	}

	contracts := state.Contracts

	initialValues := map[string]string{
		"Image": image,
	}

	for _, name := range state.ContractDefs["http-service"] {
		contract, ok := contracts[name]
		if !ok {
			cli.Fatalf("Contract %q not defined but was listed in http-service contract defs.")
		}
		run := NewContractRun(contract, initialValues)
		if err := run.Execute(); err != nil {
			cli.Fatalf("Contract %q failed: %s", name, err)
		}
	}

	cli.Success()
}

// ContractRun is a single execution of a contract. It is also the struct passed
// in when resolving templated values in the contract definition YAML.
type ContractRun struct {
	Contract deploy.Contract
	// GlobalValues is shared between all servers started by the contract.
	// Once written, no item in GlobalValues should ever be changed.
	GlobalValues map[string]string
	// Values contains the resolved values for a specific contract. Typically
	// these are defined as Go text templated values in the contract definition
	// YAML.
	Values map[string]string
	// Servers contains all the started servers for this contract run.
	Servers       map[string]*StartedServer
	Preconditions []string
	Checks        []string
}

func NewContractRun(contract deploy.Contract, initialValues map[string]string) *ContractRun {
	if initialValues == nil {
		initialValues = map[string]string{}
	}
	return &ContractRun{
		Contract:     contract,
		GlobalValues: initialValues,
		Servers:      map[string]*StartedServer{},
	}
}

func (r *ContractRun) Execute() error {
	c := r.Contract
	cli.Logf("** ==> Running contract: %q**", c.Name)

	// First make sure all the necessary servers are started, in the correct order.
	for _, serverName := range c.StartServers {
		if err := r.StartServer(serverName); err != nil {
			return err
		}
	}

	// Second, resolve templated Values map in the contract, in light of the
	// started servers and everything else inside the ContractRun at this point.
	var values map[string]string
	if err := yaml.InjectTemplatePipeline(c.Values, &values, r); err != nil {
		return err
	}
	r.Values = values

	// Third, resolve all the other templated values in the contract using the special
	// Values map. (This is done in 2 stages to make the contracts significantly more
	// readable.
	if err := yaml.InjectTemplatePipeline(c, &c, values); err != nil {
		return err
	}

	// Next execute all the precondition checks to ensure we can meaningfully
	// run the main contract checks.
	for _, p := range c.Preconditions {
		if err := r.ExecuteCheck(p); err != nil {
			return fmt.Errorf("Precondition %q failed: %s", p.String(), err)
		}
		cli.Verbosef(" ==> Precondition **%s** passed.", p)
	}

	return nil
}

func (r *ContractRun) ExecuteCheck(c deploy.Check) error {
	if err := c.Validate(); err != nil {
		return err
	}
	var err error
	switch {
	default:
		return fmt.Errorf("Check %q is invalid: %s", c, err)
	case c.Shell != "":
		return r.ExecuteShellCheck(c.Shell, c.ExitCode)
	case c.GET != "":
		return r.ExecuteGETCheck(c)
	}
}

func (r *ContractRun) ExecuteShellCheck(command string, successExitCode int) error {
	code := cmd.ExitCode("/bin/sh", "-c", command)
	if code != successExitCode {
		return fmt.Errorf("got exit code %d; want %d", code, successExitCode)
	}
	return nil
}

func (r *ContractRun) ExecuteGETCheck(c deploy.Check) error {
	return nil
}

func (r *ContractRun) StartServer(serverName string) error {
	c := r.Contract
	s, ok := c.Servers[serverName]
	if !ok {
		return fmt.Errorf("Contract %q specifies %s in StartServers, but no server with that name exists", c.Name, serverName)
	}
	resolvedServer, err := r.ResolveServer(s)
	if err != nil {
		return err
	}
	server, err := resolvedServer.Start()
	if err != nil {
		return err
	}
	cli.Verbosef("Started server %q (%s) as %s", serverName, resolvedServer.Docker.Image, server.Container.CID())
	cli.AddCleanupTask(func() error {
		if err := server.Container.KillIfRunning(); err != nil {
			cli.Logf("Failed to stop %q container (%s)", serverName, server.Container.CID())
		} else {
			cli.Verbosef("Stopped %q container (%s)", serverName, server.Container.CID())
		}
		return err
	})
	r.Servers[serverName] = server
	return nil
}

// ResolvedServer is a *deploy.TestServer whose templated values
// have all been expanded, and is thus ready to be run.
type ResolvedServer deploy.TestServer

type StartedServer struct {
	*ResolvedServer
	Container docker.Container
}

// ResolveServer fleshes out all templated values in the server in the
// context of the current contract run.
func (r *ContractRun) ResolveServer(s deploy.TestServer) (*ResolvedServer, error) {
	cli.Verbosef("Resolving values for server %q", s.Name)
	for k, v := range s.DefaultValues {
		// Don't use default value if we already have that value in the global agglomeration.
		if v, ok := r.GlobalValues[k]; ok {
			cli.Verbosef(" ==> %s=%q (already set)", k, v)
			continue
		}
		v = tools.TrimWhitespace(v)
		if !strings.HasPrefix(v, "$(") {
			r.GlobalValues[k] = v
			cli.Verbosef(" ==> %s=%q", k, v)
			continue
		}
		v = trimPrefixAndSuffix(v, "$(", ")")
		result := cmd.Stdout("/bin/sh", "-c", v)
		r.GlobalValues[k] = result
		cli.Verbosef(" ==> %s=%q (%s)", k, result, v)
	}

	err := yaml.InjectTemplatePipeline(s, &s, r.GlobalValues)
	if err != nil {
		return nil, err
	}
	rs := ResolvedServer(s)
	return &rs, nil
}

func (s *ResolvedServer) Start() (*StartedServer, error) {
	run, err := s.MakeDockerRun()
	if err != nil {
		return nil, err
	}
	run.StdoutFile = "/dev/null"
	run.StderrFile = "/dev/null"
	container, err := run.Start()
	if err != nil {
		return nil, err
	}
	startedServer := &StartedServer{s, container}

	return startedServer, nil
}

func (s *ResolvedServer) MakeDockerRun() (*docker.Run, error) {
	d := s.Docker
	run := docker.NewRun(d.Image)
	for k, v := range d.Env {
		run.AddEnv(k, v)
	}
	run.Args = d.Args
	return run, nil
}

func trimPrefixAndSuffix(s, prefix, suffix string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s, prefix), suffix)
}

func within(d time.Duration, f func() bool) int {
	start := time.Now()
	end := start.Add(d)
	p := cli.BeginProgress("Polling")
	for {
		if f() {
			p.Done("Success!")
			return 0
		}
		if time.Now().After(end) {
			break
		}
		p.Increment()
		time.Sleep(time.Second)
	}
	p.Done("Timeout")
	return 1
}
