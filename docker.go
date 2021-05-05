package docker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/coredns/caddy"

	"github.com/coredns/coredns/plugin/metrics"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/miekg/dns"
)

type containerInfo struct {
	container *types.ContainerJSON
	address   net.IP
	domains   []string // resolved domain
}

type containerInfoMap map[string]*containerInfo

type containerDomainResolver interface {
	// return domains without trailing dot
	resolve(container *types.ContainerJSON) ([]string, error)
}

// Discovery is a plugin that conforms to the coredns plugin interface
type Discovery struct {
	Next             plugin.Handler
	dockerEndpoint   string
	resolvers        []containerDomainResolver
	dockerClient     *client.Client
	containerInfoMap containerInfoMap
	TTL              uint32
	Zone             string
	Zones            []string
	caddy            *caddy.Controller
}

// NewDiscovery constructs a new DockerDiscovery object
func NewDiscovery(c *caddy.Controller, dockerEndpoint string) Discovery {
	return Discovery{
		dockerEndpoint:   dockerEndpoint,
		containerInfoMap: make(containerInfoMap),
		caddy:            c,
	}
}

func (dd Discovery) resolveDomainsByContainer(container *types.ContainerJSON) ([]string, error) {
	var domains []string
	for _, resolver := range dd.resolvers {
		var d, err = resolver.resolve(container)
		if err != nil {
			log.Infof("[zone/%s] Error resolving container domains %s", dd.Zone, err)
		}
		domains = append(domains, d...)
	}

	return domains, nil
}

func (dd Discovery) containerInfoByDomain(requestName string) (*containerInfo, error) {
	for _, containerInfoData := range dd.containerInfoMap {
		for _, d := range containerInfoData.domains {
			if fmt.Sprintf("%s.", d) == requestName { // qualified domain name must be specified with a trailing dot
				return containerInfoData, nil
			}
		}
	}

	return nil, nil
}

// ServeDNS implements plugin.Handler
func (dd Discovery) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	state := request.Request{W: w, Req: r}

	zone := plugin.Zones(dd.Zones).Matches(state.Name())
	if zone == "" {
		return plugin.NextOrFailure(dd.Name(), dd.Next, ctx, w, r)
	}

	var answers []dns.RR
	switch state.QType() {
	case dns.TypeA:
		containerInfoData, _ := dd.containerInfoByDomain(state.QName())
		if containerInfoData != nil {
			metricsDockerSuccessCountVec.WithLabelValues(metrics.WithServer(ctx), zone, state.QName()).Inc()
			metricsDockerSuccessCount.Inc()
			log.Debugf("[zone/%s] A Found ip %v for zone %s and host %s", dd.Zone, containerInfoData.address, zone, state.QName())
			answers = dd.a(state, []net.IP{containerInfoData.address})
		} else {
			metricsDockerFailureCountVec.WithLabelValues(metrics.WithServer(ctx), zone, state.QName()).Inc()
			metricsDockerFailureCount.Inc()
		}
	}

	if len(answers) == 0 {
		return plugin.NextOrFailure(dd.Name(), dd.Next, ctx, w, r)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, true, true
	m.Answer = answers

	state.SizeAndDo(m)
	m = state.Scrub(m)
	err := w.WriteMsg(m)
	if err != nil {
		log.Infof("[zone/%s] Error: %s", dd.Zone, err.Error())
	}
	return dns.RcodeSuccess, nil
}

// Name implements plugin.Handler
func (dd Discovery) Name() string {
	return pluginName
}

func (dd Discovery) getContainerAddress(container *types.ContainerJSON) (net.IP, error) {
	for {
		if container.NetworkSettings.IPAddress != "" {
			return net.ParseIP(container.NetworkSettings.IPAddress), nil
		}

		networkMode := container.HostConfig.NetworkMode

		// TODO: Deal with containers run with host ip (--net=host)
		// if networkMode == "host" {
		// 	log.Println("Container uses host network")
		// 	return nil, nil
		// }

		if strings.HasPrefix(string(networkMode), "container:") {
			log.Debugf("[zone/%s] Container %s is in another container's network namspace", dd.Zone, container.ID[:12])
			otherID := container.HostConfig.NetworkMode[len("container:"):]
			var err error
			*container, err = dd.dockerClient.ContainerInspect(context.TODO(), string(otherID))
			if err != nil {
				return nil, err
			}
			continue
		} else {
			network, ok := container.NetworkSettings.Networks[string(networkMode)]
			if !ok { // sometime while "network:disconnect" event fire
				return nil, fmt.Errorf("unable to find network settings for the network %s", networkMode)
			}

			return net.ParseIP(network.IPAddress), nil // ParseIP return nil when IPAddress equals ""
		}
	}
}

func (dd Discovery) updateContainerInfo(container *types.ContainerJSON) error {
	_, isExist := dd.containerInfoMap[container.ID]
	containerAddress, err := dd.getContainerAddress(container)
	if isExist { // remove previous resolved container info
		delete(dd.containerInfoMap, container.ID)
	}

	if err != nil || containerAddress == nil {
		log.Debugf("[zone/%s] Remove container entry %s (%s)", dd.Zone, normalizeContainerName(container), container.ID[:12])
		return err
	}

	domains, _ := dd.resolveDomainsByContainer(container)
	if len(domains) > 0 {
		dd.containerInfoMap[container.ID] = &containerInfo{
			container: container,
			address:   containerAddress,
			domains:   domains,
		}

		if !isExist {
			log.Debugf("[zone/%s] A dd entry of container %s (%s). IP: %v, Domains: [%s]", dd.Zone, normalizeContainerName(container), container.ID[:12], containerAddress, strings.Join(domains, ", "))
		}
	} else if isExist {
		log.Debugf("[zone/%s] Remove container entry %s (%s)", dd.Zone, normalizeContainerName(container), container.ID[:12])
	}
	return nil
}

func (dd Discovery) removeContainerInfo(containerID string) error {
	containerInfoData, ok := dd.containerInfoMap[containerID]
	if !ok {
		log.Debugf("[zone/%s] No entry associated with the container %s", dd.Zone, containerID[:12])
		return nil
	}
	log.Debugf("[zone/%s] Deleting entry %s (%s)", dd.Zone, normalizeContainerName(containerInfoData.container), containerInfoData.container.ID[:12])
	delete(dd.containerInfoMap, containerID)

	return nil
}

func (dd Discovery) start() error {
	log.Debugf("[zone/%s] start", dd.Zone)
	containers, err := dd.dockerClient.ContainerList(context.Background(), types.ContainerListOptions{All: false})
	if err != nil {
		return err
	}

	for i := range containers {
		container, err := dd.dockerClient.ContainerInspect(context.Background(), containers[i].ID)
		if err != nil {
			// TODO err
		}
		if err = dd.updateContainerInfo(&container); err != nil {
			log.Errorf("[zone/%s] Error adding A record for container %s: %+v", dd.Zone, container.ID[:12], err)
		}
	}

	metricsDockerContainers.WithLabelValues().Set(float64(len(dd.containerInfoMap)))
	metricsmetricsDockerDomainsUpdate(&dd)

	filter := filters.NewArgs()

	filter.Add("type", "container")
	filter.Add("event", "start")
	filter.Add("event", "die")

	filter.Add("type", "network")
	filter.Add("event", "connect")
	filter.Add("event", "disconnect")

	event, errChan := dd.dockerClient.Events(context.Background(), types.EventsOptions{Filters: filter})

forLoop:
	for {
		select {
		case err = <-errChan:
			// TODO err
			// return err
			log.Errorf("[zone/%s] docker client event litener error acquired: %+v", dd.Zone, err)
			break forLoop
		case msg := <-event:
			go dockerEventHandler(&dd, msg)
		}
	}

	// for msg := range events {
	// }

	return errors.New("docker event loop closed")
}

// a takes a slice of net.IPs and returns a slice of A RRs.
func (dd Discovery) a(state request.Request, ips []net.IP) []dns.RR {
	var answers []dns.RR
	for _, ip := range ips {
		answers = append(answers, &dns.A{
			Hdr: dns.RR_Header{
				Name:   state.QName(),
				Ttl:    dd.TTL,
				Class:  dns.ClassINET,
				Rrtype: dns.TypeA,
			},
			A: ip.To4(),
		})
	}
	return answers
}

func dockerEventHandler(dd *Discovery, msg events.Message) {
	event := fmt.Sprintf("%s:%s", msg.Type, msg.Action)
	switch event {
	case "container:start":
		log.Debugf("[zone/%s] New container #%s spawned. Attempt to add A record for it", dd.Zone, msg.Actor.ID[:12])
		container, err := dd.dockerClient.ContainerInspect(context.Background(), msg.Actor.ID)
		if err != nil {
			log.Errorf("[zone/%s] Container #%s event %s: %s", dd.Zone, msg.Actor.ID[:12], event, err)
			return
		}
		if err = dd.updateContainerInfo(&container); err != nil {
			log.Errorf("[zone/%s] Error adding A record for container #%s: %s", dd.Zone, container.ID[:12], err)
		}
	case "container:die":
		log.Debugf("[zone/%s] Container %%s being stopped. Attempt to remove its A record from the DNS", dd.Zone, msg.Actor.ID[:12])
		if err := dd.removeContainerInfo(msg.Actor.ID); err != nil {
			log.Errorf("[zone/%s] Error deleting A record for container: %s: %s", dd.Zone, msg.Actor.ID[:12], err)
		}
	case "network:connect":
		// take a look https://gist.github.com/josefkarasek/be9bac36921f7bc9a61df23451594fbf for example of same event's types attributes
		log.Debugf("[zone/%s] Container #%s being connected to network %s.", dd.Zone, msg.Actor.Attributes["container"][:12], msg.Actor.Attributes["name"])

		container, err := dd.dockerClient.ContainerInspect(context.Background(), msg.Actor.Attributes["container"])
		if err != nil {
			log.Errorf("[zone/%s] Event error %s #%s: %s", dd.Zone, event, msg.Actor.Attributes["container"][:12], err)
			return
		}
		if err = dd.updateContainerInfo(&container); err != nil {
			log.Errorf("[zone/%s] Error adding A record for container %s: %s", dd.Zone, container.ID[:12], err)
		}
	case "network:disconnect":
		log.Debugf("[zone/%s] Container %s being disconnected from network %s", dd.Zone, msg.Actor.Attributes["container"][:12], msg.Actor.Attributes["name"])

		container, err := dd.dockerClient.ContainerInspect(context.Background(), msg.Actor.Attributes["container"])
		if err != nil {
			log.Errorf("[zone/%s] Event error %s #%s: %s", dd.Zone, event, msg.Actor.Attributes["container"][:12], err)
			return
		}
		if err = dd.updateContainerInfo(&container); err != nil {
			log.Errorf("[zone/%s] Error adding A record for container %s: %s", dd.Zone, container.ID[:12], err)
		}
	}
	metricsDockerContainers.WithLabelValues().Set(float64(len(dd.containerInfoMap)))
	metricsmetricsDockerDomainsUpdate(dd)
}
