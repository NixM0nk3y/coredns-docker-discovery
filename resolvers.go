package dockerdiscovery

import (
	"fmt"
	"strings"

	"github.com/docker/docker/api/types"
)

func normalizeContainerName(container *types.ContainerJSON) string {
	return strings.TrimLeft(container.Name, "/")
}

// resolvers implements ContainerDomainResolver

type SubDomainContainerNameResolver struct {
	domain string
}

func (resolver SubDomainContainerNameResolver) resolve(container *types.ContainerJSON) ([]string, error) {
	var domains []string
	domains = append(domains, fmt.Sprintf("%s.%s", normalizeContainerName(container), resolver.domain))
	return domains, nil
}

type SubDomainHostResolver struct {
	domain string
}

func (resolver SubDomainHostResolver) resolve(container *types.ContainerJSON) ([]string, error) {
	var domains []string
	domains = append(domains, fmt.Sprintf("%s.%s", container.Config.Hostname, resolver.domain))
	return domains, nil
}

type LabelResolver struct {
	hostLabel string
}

func (resolver LabelResolver) resolve(container *types.ContainerJSON) ([]string, error) {
	var domains []string

	for label, value := range container.Config.Labels {
		if label == resolver.hostLabel {
			domains = append(domains, value)
			break
		}
	}

	return domains, nil
}

type NetworkAliasesResolver struct {
	network string
}

func (resolver NetworkAliasesResolver) resolve(container *types.ContainerJSON) ([]string, error) {
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
