package cli

import (
	"flag"

	"github.com/opentable/sous/ext/docker"
	"github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/cmdr"
)

// SousBuild is the command description for `sous build`
// Implements cmdr.Command, cmdr.Executor and cmdr.AddFlags
type SousBuild struct {
	Sous          *Sous
	DockerClient  LocalDockerClient
	Config        LocalSousConfig
	WDShell       LocalWorkDirShell
	ScratchShell  ScratchDirShell
	SourceContext *sous.SourceContext
	BuildContext  *sous.BuildContext
	Builder       sous.Builder
	flags         struct {
		config              sous.BuildConfig
		target              string
		rebuild, rebuildAll bool
	}
}

func init() { TopLevelCommands["build"] = &SousBuild{} }

const sousBuildHelp = `
build your project

build builds the project in your current directory by default. If you pass it a
path, it will instead build the project at that path.

args: [path]
`

// Help returns the help string for this command
func (*SousBuild) Help() string { return sousBuildHelp }

// AddFlags adds flags for sous build
func (sb *SousBuild) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&sb.flags.config.Repo, "repo",
		"The authoritive repository for this project")
	fs.StringVar(&sb.flags.config.Offset, "offset",
		"The offset within repository for this project")
	fs.StringVar(&sb.flags.config.Tag, "tag",
		"The tag to build for this project - should conform to semver (e.g. 1.2.3-pre)")
	fs.StringVar(&sb.flags.config.Repo, "revision",
		"The revision of this project to build - a git digest")
	fs.BoolVar(&sb.flags.config.Strict, "strict",
		"If advisories would be added to the build, fail instead")
	fs.BoolVar(&sb.flags.config.ForceClone, "force-clone",
		"Ignore the current directory and work in a shallow clone of the project")
}

// Execute fulfills the cmdr.Executor interface
func (sb *SousBuild) Execute(args []string) cmdr.Result {
	if len(args) != 0 {
		path := args[0]
		if err := sb.WDShell.CD(path); err != nil {
			return cmdr.EnsureErrorResult(err)
		}
	}

	bp := docker.NewDockerfileBuildpack()
	dr, err := bp.Detect(sb.BuildContext)
	if err != nil {
		return cmdr.EnsureErrorResult(err)
	}

	mgr := &BuildManager{
		BuildConfig:  &sb.flags.config,
		BuildContext: sb.BuildContext,
		BuildPack:    bp,
		Detect:       dr,
		Builder:      sb.Builder,
		Registrar:    sb.Builder,
	}

	result, err := mgr.Build()

	if err != nil {
		return cmdr.EnsureErrorResult(err)
	}

	//	return Success(result)
	return Success(result)
}
