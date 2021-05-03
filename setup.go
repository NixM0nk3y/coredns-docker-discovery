package dockerdiscovery

import (
	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/docker/docker/client"
	dockerClient "github.com/docker/docker/client"
)

const defaultDockerDomain = "docker.local"

func init() {
	caddy.RegisterPlugin("docker", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

// TODO(kevinjqiu): add docker endpoint verification
func createPlugin(c *caddy.Controller) (DockerDiscovery, error) {
	dd := NewDockerDiscovery(client.DefaultDockerHost)
	labelResolver := &LabelResolver{hostLabel: "coredns.dockerdiscovery.host"}
	dd.resolvers = append(dd.resolvers, labelResolver)

	for c.Next() {
		args := c.RemainingArgs()
		if len(args) == 1 {
			dd.dockerEndpoint = args[0]
		}

		if len(args) > 1 {
			return dd, c.ArgErr()
		}

		for c.NextBlock() {
			var value = c.Val()
			switch value {
			case "domain":
				var resolver = &subDomainContainerNameResolver{
					domain: defaultDockerDomain,
				}
				dd.resolvers = append(dd.resolvers, resolver)
				if !c.NextArg() {
					return dd, c.ArgErr()
				}
				resolver.domain = c.Val()
			case "hostname_domain":
				var resolver = &subDomainHostResolver{
					domain: defaultDockerDomain,
				}
				dd.resolvers = append(dd.resolvers, resolver)
				if !c.NextArg() {
					return dd, c.ArgErr()
				}
				resolver.domain = c.Val()
			case "network_aliases":
				var resolver = &networkAliasesResolver{
					network: "",
				}
				dd.resolvers = append(dd.resolvers, resolver)
				if !c.NextArg() {
					return dd, c.ArgErr()
				}
				resolver.network = c.Val()
			case "label":
				if !c.NextArg() {
					return dd, c.ArgErr()
				}
				labelResolver.hostLabel = c.Val()
			default:
				return dd, c.Errf("unknown property: '%s'", c.Val())
			}
		}
	}
	var err error

	// todo add options for tls connections and other
	dd.dockerClient, err = dockerClient.NewClientWithOpts(dockerClient.WithHost(dd.dockerEndpoint))
	if err != nil {
		return dd, err
	}

	go dd.start()
	return dd, nil
}

func setup(c *caddy.Controller) error {
	dd, err := createPlugin(c)
	if err != nil {
		return err
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		dd.Next = next
		return dd
	})
	return nil
}
