package docker

import (
	"fmt"
	"net"
	"testing"

	"github.com/coredns/caddy"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
)

type setupDockerDiscoveryTestCase struct {
	configBlock            string
	expectedDockerEndpoint string
	expectedDockerDomain   string
}

func TestSetupDockerDiscovery(t *testing.T) {
	testCases := []setupDockerDiscoveryTestCase{
		{
			"docker",
			client.DefaultDockerHost,
			defaultDockerDomain,
		},
		{
			fmt.Sprintf("docker %s.backup", client.DefaultDockerHost),
			fmt.Sprintf("%s.backup", client.DefaultDockerHost),
			defaultDockerDomain,
		},
		{
			`docker {
				hostname_domain example.org.
			}`,
			client.DefaultDockerHost,
			"example.org.",
		},
		{

			fmt.Sprintf(`docker %s {
				hostname_domain home.example.org.
			}`, client.DefaultDockerHost),
			client.DefaultDockerHost,
			"home.example.org.",
		},
	}

	for _, tc := range testCases {
		c := caddy.NewTestController("dns", tc.configBlock)
		dd, err := createPlugin(c)
		assert.Nil(t, err, tc.configBlock, tc.expectedDockerDomain, tc.expectedDockerEndpoint)
		assert.Equal(t, dd.dockerEndpoint, tc.expectedDockerEndpoint)
	}

	c := caddy.NewTestController("dns",
		fmt.Sprintf(`docker %s {
		hostname_domain home.example.org
		domain docker.loc
		network_aliases my_project_network_name
	}`, client.DefaultDockerHost))
	dd, err := createPlugin(c)
	assert.Nil(t, err)

	networks := make(map[string]*network.EndpointSettings)
	var aliases = []string{"myproject.loc"}

	networks["my_project_network_name"] = &network.EndpointSettings{
		Aliases: aliases,
	}
	var address = net.ParseIP("192.11.0.1")

	var containerData = &types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:   "fa155d6fd141e29256c286070d2d44b3f45f1e46822578f1e7d66c1e7981e6c7",
			Name: "evil_ptolemy",
		},
		Config: &container.Config{
			Hostname: "nginx",
			Labels:   map[string]string{"coredns.dockerdiscovery.host": "label-host.loc"},
		},
		NetworkSettings: &types.NetworkSettings{
			DefaultNetworkSettings: types.DefaultNetworkSettings{
				IPAddress: address.String(),
			},
			Networks: networks,
		},
	}

	err = dd.updateContainerInfo(containerData)
	assert.Nil(t, err)

	containerInfoData, err := dd.containerInfoByDomain("myproject.loc.")
	assert.Nil(t, err)
	assert.NotNil(t, containerInfoData)
	assert.NotNil(t, containerInfoData.address)
	assert.Equal(t, containerInfoData.address, address)

	containerInfoData, _ = dd.containerInfoByDomain("wrong.loc.")
	assert.Nil(t, containerInfoData)

	containerInfoData, err = dd.containerInfoByDomain("nginx.home.example.org.")
	assert.Nil(t, err)
	assert.NotNil(t, containerInfoData)

	containerInfoData, _ = dd.containerInfoByDomain("wrong.home.example.org.")
	assert.Nil(t, containerInfoData)

	containerInfoData, err = dd.containerInfoByDomain("label-host.loc.")
	assert.Nil(t, err)
	assert.NotNil(t, containerInfoData)

	containerInfoData, err = dd.containerInfoByDomain(fmt.Sprintf("%s.docker.loc.", containerData.Name))
	assert.Nil(t, err)
	assert.NotNil(t, containerInfoData)
	assert.Equal(t, containerData.Name, containerInfoData.container.Name)
}
