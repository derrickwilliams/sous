package docker

import (
	"os"
	"testing"

	"github.com/opentable/sous/lib"
	"github.com/opentable/sous/util/shell"
	"github.com/stretchr/testify/assert"
)

func TestSplitBuilder_BuildBuild(t *testing.T) {
	sh, ctl := shell.NewTestShell()

	builder := splitBuilder{
		context: &sous.BuildContext{
			Sh: sh,
		},
		detected: &sous.DetectResult{
			Data: detectData{},
		},
	}

	_, cctl := ctl.CmdFor("docker", "build")
	cctl.ResultSuccess("Successfully built cabba9edeadbeef", "")

	err := builder.buildBuild()
	assert.NoError(t, err)

	assert.Equal(t, builder.buildImageID, "cabba9edeadbeef")
}

func TestSplitBuilder_SetupTempdir(t *testing.T) {
	builder := splitBuilder{}
	assert.NoError(t, builder.setupTempdir())

	fi, err := os.Stat(builder.tempDir)
	assert.NoError(t, err)
	assert.True(t, fi.IsDir())

	fi, err = os.Stat(builder.buildDir)
	assert.NoError(t, err)
	assert.True(t, fi.IsDir())
}

func TestSplitBuilder_ExtractRunSpec(t *testing.T) {
	sh, ctl := shell.NewTestShell()

	builder := splitBuilder{
		context: &sous.BuildContext{
			Sh: sh,
		},
		buildContainerID: "qwerqwerqwer",
		tempDir:          "testdata/splitbuilder",
		detected: &sous.DetectResult{
			Data: detectData{
				RunImageSpecPath: "/housekeeping/runspec.json",
			},
		},
	}

	assert.NoError(t, builder.extractRunSpec())
	assert.Len(t, builder.RunSpec.Images, 3)
	assert.Len(t, ctl.CmdsLike("docker", "cp"), 1)
}

func TestSplitBuilder_ValidateRunspec(t *testing.T) {
	builder := splitBuilder{
		RunSpec: &MultiImageRunSpec{
			SplitImageRunSpec: &SplitImageRunSpec{
				Files: []sbmInstall{{Source: sbmFile{"a"}, Destination: sbmFile{"a"}}},
			},
			Images: []SplitImageRunSpec{{
				Files: []sbmInstall{{Source: sbmFile{"a"}, Destination: sbmFile{"a"}}},
			}},
		},
	}
	assert.Error(t, builder.validateRunSpec(), "should have returned error from invalid runspec")
}

func TestSplitBuilder_ConstructSubBuilders(t *testing.T) {
	builder := splitBuilder{
		RunSpec: &MultiImageRunSpec{
			Images: []SplitImageRunSpec{
				{
					Files: []sbmInstall{{Source: sbmFile{"a"}, Destination: sbmFile{"a"}}},
				},
				{
					Files: []sbmInstall{{Source: sbmFile{"a"}, Destination: sbmFile{"a"}}},
				},
			},
		},
	}

	assert.NoError(t, builder.constructImageBuilders())
	assert.Len(t, builder.subBuilders, 2)
}

func TestSplitBuilder_Result(t *testing.T) {
	builder := splitBuilder{
		context: &sous.BuildContext{},
	}
	builder.subBuilders = []*runnableBuilder{{
		RunSpec:      SplitImageRunSpec{Kind: "tester"},
		splitBuilder: &builder,
	}}
	res := builder.result()
	assert.Len(t, res.Products, 2)
}
