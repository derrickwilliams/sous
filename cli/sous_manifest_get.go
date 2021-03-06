package cli

import (
	"flag"

	"github.com/davecgh/go-spew/spew"
	"github.com/opentable/sous/config"
	"github.com/opentable/sous/graph"
	sous "github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/cmdr"
	"github.com/opentable/sous/util/logging"
	"github.com/opentable/sous/util/yaml"
	"github.com/pkg/errors"
)

type SousManifestGet struct {
	config.DeployFilterFlags
	*sous.ResolveFilter
	graph.TargetManifestID
	graph.HTTPClient
	*logging.LogSet
	graph.OutWriter
}

func init() { ManifestSubcommands["get"] = &SousManifestGet{} }

const sousManifestGetHelp = `query deployment manifest`

func (*SousManifestGet) Help() string { return sousManifestHelp }

func (smg *SousManifestGet) AddFlags(fs *flag.FlagSet) {
	MustAddFlags(fs, &smg.DeployFilterFlags, ManifestFilterFlagsHelp)
}

func (smg *SousManifestGet) RegisterOn(psy Addable) {
	psy.Add(&smg.DeployFilterFlags)
}

func (smg *SousManifestGet) Execute(args []string) cmdr.Result {
	mani := sous.Manifest{}
	_, err := smg.HTTPClient.Retrieve("./manifests", smg.TargetManifestID.QueryMap(), &mani, nil)

	if err != nil {
		return EnsureErrorResult(errors.Errorf("No manifest matched by %v yet. See `sous init`", smg.ResolveFilter))
	}
	smg.Vomitf(spew.Sdump(mani))

	yml, err := yaml.Marshal(mani)
	if err != nil {
		return EnsureErrorResult(err)
	}
	smg.OutWriter.Write(yml)
	return cmdr.Success()
}
