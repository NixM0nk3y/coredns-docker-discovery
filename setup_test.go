package dockerdiscovery

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

	var container = &types.ContainerJSON{
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

	err = dd.updateContainerInfo(container)
	assert.Nil(t, err)

	containerInfo, err := dd.containerInfoByDomain("myproject.loc.")
	assert.Nil(t, err)
	assert.NotNil(t, containerInfo)
	assert.NotNil(t, containerInfo.address)
	assert.Equal(t, containerInfo.address, address)

	containerInfo, _ = dd.containerInfoByDomain("wrong.loc.")
	assert.Nil(t, containerInfo)

	containerInfo, err = dd.containerInfoByDomain("nginx.home.example.org.")
	assert.Nil(t, err)
	assert.NotNil(t, containerInfo)

	containerInfo, _ = dd.containerInfoByDomain("wrong.home.example.org.")
	assert.Nil(t, containerInfo)

	containerInfo, err = dd.containerInfoByDomain("label-host.loc.")
	assert.Nil(t, err)
	assert.NotNil(t, containerInfo)

	containerInfo, err = dd.containerInfoByDomain(fmt.Sprintf("%s.docker.loc.", container.Name))
	assert.Nil(t, err)
	assert.NotNil(t, containerInfo)
	assert.Equal(t, container.Name, containerInfo.container.Name)
}
