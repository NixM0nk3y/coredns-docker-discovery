package docker

import (
	"github.com/coredns/coredns/plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// MetricsDockerSuccessCount report the number of times we've seen a localhost.<domain> query.
	metricsDockerSuccessCountVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "requests_success",
		Help:      "Counter of success docker hosts requests.",
	}, []string{"server", "zone", "domain"})

	// metricsDockerFailureCountVec report the number of times we've seen a localhost.<domain> query.
	metricsDockerFailureCountVec = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "requests_failures",
		Help:      "Counter of success docker hosts requests.",
	}, []string{"server", "zone", "domain"})

	// metricsDockerSuccessCount report the number of times we've seen a localhost.<domain> query.
	metricsDockerSuccessCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "requests_success_total",
		Help:      "Counter of failure docker hosts requests.",
	})

	// MetricsDockerFailureCount report the number of times we've seen a localhost.<domain> query.
	metricsDockerFailureCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "requests_failures_total",
		Help:      "Counter of failure docker hosts requests.",
	})

	// metricsDockerContainers is the combined number of docker containers entries.
	metricsDockerContainers = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "containers_count",
		Help:      "The combined number of docker containers entries.",
	}, []string{})

	// metricsDockerDomains is the combined number of docker domains entries.
	metricsDockerDomains = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: pluginName,
		Name:      "domains_count",
		Help:      "The combined number of docker domains entries.",
	}, []string{})
)

func metricsmetricsDockerDomainsUpdate(dd *Discovery) {
	cnt := 0
	for i := range dd.containerInfoMap {
		cnt += len(dd.containerInfoMap[i].domains)
	}
	metricsDockerDomains.WithLabelValues().Set(float64(cnt))
}
