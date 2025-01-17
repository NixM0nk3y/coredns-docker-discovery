package docker

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
)

func normalizeContainerName(container *types.ContainerJSON) string {
	return strings.TrimLeft(container.Name, "/")
}

// resolvers implements ContainerDomainResolver

type subDomainContainerNameResolver struct {
	domain string
}

func (resolver subDomainContainerNameResolver) resolve(container *types.ContainerJSON) ([]string, error) {
	var domains []string
	domains = append(domains, fmt.Sprintf("%s.%s", normalizeContainerName(container), resolver.domain))
	return domains, nil
}

type subDomainHostResolver struct {
	domain string
}

func (resolver subDomainHostResolver) resolve(container *types.ContainerJSON) ([]string, error) {
	var domains []string
	domains = append(domains, fmt.Sprintf("%s.%s", container.Config.Hostname, resolver.domain))
	return domains, nil
}

type labelResolver struct {
	hostLabel string
}

func (resolver labelResolver) resolve(container *types.ContainerJSON) ([]string, error) {
	var domains []string

	for label, value := range container.Config.Labels {
		if label == resolver.hostLabel {
			domains = append(domains, value)
			break
		}
	}

	return domains, nil
}

type networkAliasesResolver struct {
	network string
}

func (resolver networkAliasesResolver) resolve(container *types.ContainerJSON) ([]string, error) {
	var domains []string

	if resolver.network != "" {
		network, ok := container.NetworkSettings.Networks[resolver.network]
		if ok {
			domains = append(domains, network.Aliases...)
		}
	} else {
		for _, network := range container.NetworkSettings.Networks {
			domains = append(domains, network.Aliases...)
		}
	}

	return domains, nil
}
