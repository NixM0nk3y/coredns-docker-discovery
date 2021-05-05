package docker

import (
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/docker/docker/client"
	dockerClient "github.com/docker/docker/client"
)

const defaultDockerDomain = "docker.local"
const defaultDockerHostDomain = "docker-host.local"
const defaultDomainTTL uint32 = 3600
const pluginName = "docker"

var log = clog.NewWithPlugin(pluginName)

func init() {
	plugin.Register(pluginName, setup)
}

// TODO(kevinjqiu): add docker endpoint verification
func createPlugin(c *caddy.Controller) (Discovery, error) {
	dd := NewDiscovery(c, client.DefaultDockerHost)
	labelResolvers := &labelResolver{hostLabel: "coredns.dockerdiscovery.host"}
	dd.resolvers = append(dd.resolvers, labelResolvers)
	dd.TTL = defaultDomainTTL

	dd.Zone = dnsserver.GetConfig(c).Zone
	dd.Zones = append(dd.Zones, dd.Zone)

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
				dd.Zones = append(dd.Zones, resolver.domain)
			case "hostname_domain":
				var resolver = &subDomainHostResolver{
					domain: defaultDockerHostDomain,
				}
				dd.resolvers = append(dd.resolvers, resolver)
				if !c.NextArg() {
					return dd, c.ArgErr()
				}
				resolver.domain = c.Val()
				dd.Zones = append(dd.Zones, resolver.domain)
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
				labelResolvers.hostLabel = c.Val()
			case "ttl":
				if !c.NextArg() {
					return dd, c.ArgErr()
				}
				val, err := strconv.Atoi(c.Val())
				if err != nil {
					return dd, c.Errf("TTL should be an uint32: '%s' - %+v", c.Val(), err)
				}
				dd.TTL = uint32(val)
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

	go func() {
		err = dd.start()
		if err != nil {
			log.Errorf("[zone/%s] processing finished with error: %+v", dd.Zone, err)
		}
	}()
	return dd, nil
}

func setup(c *caddy.Controller) error {
	dd, err := createPlugin(c)
	if err != nil {
		return err
	}

	c.OnStartup(func() error {
		return nil
	})

	c.OnFirstStartup(func() error {
		return nil
	})

	c.OnRestart(func() error {
		return nil
	})

	c.OnRestartFailed(func() error {
		return nil
	})

	c.OnShutdown(func() error {
		//log.Info("Shutting down docker discovery")
		return nil
	})

	c.OnFinalShutdown(func() error {
		//log.Info("Final Shutting down docker discovery")
		return dd.dockerClient.Close()
	})

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		dd.Next = next
		return dd
	})
	return nil
}
