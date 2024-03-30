package model

import (
	"istio.io/istio/pkg/monitoring"
)

// Metrics is an interface for capturing metrics on a per-node basis.
type Metrics interface {
	// AddMetric will add an case to the metric for the given node.
	AddMetric(metric monitoring.Metric, key string, proxyID, msg string)
}

var (

	// EndpointNoPod tracks endpoints without an associated pod. This is an error condition, since
	// we can't figure out the labels. It may be a transient problem, if endpoint is processed before
	// pod.
	EndpointNoPod = monitoring.NewGauge(
		"endpoint_no_pod",
		"Endpoints without an associated pod.",
	)

	// ProxyStatusNoService represents proxies not selected by any service
	// This can be normal - for workloads that act only as client, or are not covered by a Service.
	// It can also be an error, for example in cases the Endpoint list of a service was not updated by the time
	// the sidecar calls.
	// Updated by GetProxyServiceTargets
	ProxyStatusNoService = monitoring.NewGauge(
		"pilot_no_ip",
		"Pods not found in the endpoint table, possibly invalid.",
	)

	// ProxyStatusEndpointNotReady represents proxies found not be ready.
	// Updated by GetProxyServiceTargets. Normal condition when starting
	// an app with readiness, error if it doesn't change to 0.
	ProxyStatusEndpointNotReady = monitoring.NewGauge(
		"pilot_endpoint_not_ready",
		"Endpoint found in unready state.",
	)

	// ProxyStatusConflictOutboundListenerTCPOverTCP metric tracks number of
	// TCP listeners that conflicted with existing TCP listeners on same port
	ProxyStatusConflictOutboundListenerTCPOverTCP = monitoring.NewGauge(
		"pilot_conflict_outbound_listener_tcp_over_current_tcp",
		"Number of conflicting tcp listeners with current tcp listener.",
	)

	// ProxyStatusConflictInboundListener tracks cases of multiple inbound
	// listeners - 2 services selecting the same port of the pod.
	ProxyStatusConflictInboundListener = monitoring.NewGauge(
		"pilot_conflict_inbound_listener",
		"Number of conflicting inbound listeners.",
	)

	// DuplicatedClusters tracks duplicate clusters seen while computing CDS
	DuplicatedClusters = monitoring.NewGauge(
		"pilot_duplicate_envoy_clusters",
		"Duplicate envoy clusters caused by service entries with same hostname",
	)

	// DNSNoEndpointClusters tracks dns clusters without endpoints
	DNSNoEndpointClusters = monitoring.NewGauge(
		"pilot_dns_cluster_without_endpoints",
		"DNS clusters without endpoints caused by the endpoint field in "+
			"STRICT_DNS type cluster is not set or the corresponding subset cannot select any endpoint",
	)

	// ProxyStatusClusterNoInstances tracks clusters (services) without workloads.
	ProxyStatusClusterNoInstances = monitoring.NewGauge(
		"pilot_eds_no_instances",
		"Number of clusters without instances.",
	)

	// DuplicatedDomains tracks rejected VirtualServices due to duplicated hostname.
	DuplicatedDomains = monitoring.NewGauge(
		"pilot_vservice_dup_domain",
		"Virtual services with dup domains.",
	)

	// DuplicatedSubsets tracks duplicate subsets that we rejected while merging multiple destination rules for same host
	DuplicatedSubsets = monitoring.NewGauge(
		"pilot_destrule_subsets",
		"Duplicate subsets across destination rules for same host",
	)

	// totalVirtualServices tracks the total number of virtual service
	totalVirtualServices = monitoring.NewGauge(
		"pilot_virt_services",
		"Total virtual services known to pilot.",
	)

	// All metrics we registered.
	metrics = []monitoring.Metric{
		DNSNoEndpointClusters,
		EndpointNoPod,
		ProxyStatusNoService,
		ProxyStatusEndpointNotReady,
		ProxyStatusConflictOutboundListenerTCPOverTCP,
		ProxyStatusConflictInboundListener,
		DuplicatedClusters,
		ProxyStatusClusterNoInstances,
		DuplicatedDomains,
		DuplicatedSubsets,
	}
)
