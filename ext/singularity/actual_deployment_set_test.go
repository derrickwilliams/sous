package singularity

import (
	"testing"

	"github.com/opentable/go-singularity"
	"github.com/opentable/go-singularity/dtos"
	"github.com/opentable/sous/lib"
	"github.com/opentable/swaggering"
	"github.com/stretchr/testify/assert"
)

func TestGetDepSetWorks(t *testing.T) {
	assert := assert.New(t)

	whip := make(map[string]swaggering.DummyControl)

	reg := sous.NewDummyRegistry()
	client := sous.NewDummyRectificationClient()
	dep := deployer{client,
		func(url string) *singularity.Client {
			cl, co := singularity.NewDummyClient(url)

			co.FeedDTO(&dtos.SingularityRequestParentList{
				&dtos.SingularityRequestParent{
					RequestDeployState: &dtos.SingularityRequestDeployState{
						ActiveDeploy: &dtos.SingularityDeployMarker{
							DeployId:  "testdep",
							RequestId: "testreq",
						},
					},
					Request: &dtos.SingularityRequest{
						Id:          "testreq",
						RequestType: dtos.SingularityRequestRequestTypeSERVICE,
						Owners:      swaggering.StringList{"jlester@opentable.com"},
					},
				},
			}, nil)

			co.FeedDTO(&dtos.SingularityDeployHistoryList{
				{
					Deploy: &dtos.SingularityDeploy{
						Metadata: map[string]string{
							"com.opentable.sous.clustername": "left",
						},
						Id: "testdep",
						ContainerInfo: &dtos.SingularityContainerInfo{
							Type:   dtos.SingularityContainerInfoSingularityContainerTypeDOCKER,
							Docker: &dtos.SingularityDockerInfo{},
							Volumes: dtos.SingularityVolumeList{
								&dtos.SingularityVolume{
									HostPath:      "/onhost",
									ContainerPath: "/indocker",
									Mode:          dtos.SingularityVolumeSingularityDockerVolumeModeRW,
								},
							},
						},
						Resources: &dtos.Resources{},
					},
				}}, nil)

			whip[url] = co
			return cl
		},
	}

	clusters := sous.Clusters{"test": {BaseURL: "http://test-singularity.org/"}}
	res, err := dep.RunningDeployments(reg, clusters)
	assert.NoError(err)
	assert.NotNil(res)
}
