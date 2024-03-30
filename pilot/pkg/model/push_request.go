// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import (
	"time"

	"istio.io/istio/pkg/util/sets"
)

// PushRequest defines a request to push to proxies
// It is used to send updates to the config update debouncer and pass to the PushQueue.
type PushRequest struct {
	// Full determines whether a full push is required or not. If false, an incremental update will be sent.
	// Incremental pushes:
	// * Do not recompute the push context
	// * Do not recompute proxy state (such as ServiceInstances)
	// * Are not reported in standard metrics such as push time
	// As a result, configuration updates should never be incremental. Generally, only EDS will set this, but
	// in the future SDS will as well.
	Full bool

	// ConfigsUpdated keeps track of configs that have changed.
	// This is used as an optimization to avoid unnecessary pushes to proxies that are scoped with a Sidecar.
	// If this is empty, then all proxies will get an update.
	// Otherwise only proxies depend on these configs will get an update.
	// The kind of resources are defined in pkg/config/schemas.
	ConfigsUpdated sets.Set[ConfigKey]

	// Push stores the push context to use for the update. This may initially be nil, as we will
	// debounce changes before a PushContext is eventually created.
	Push *PushContext

	// Start represents the time a push was started. This represents the time of adding to the PushQueue.
	// Note that this does not include time spent debouncing.
	Start time.Time

	// Reason represents the reason for requesting a push. This should only be a fixed set of values,
	// to avoid unbounded cardinality in metrics. If this is not set, it may be automatically filled in later.
	// There should only be multiple reasons if the push request is the result of two distinct triggers, rather than
	// classifying a single trigger as having multiple reasons.
	Reason ReasonStats

	// Delta defines the resources that were added or removed as part of this push request.
	// This is set only on requests from the client which change the set of resources they (un)subscribe from.
	Delta ResourceDelta
}

// Merge two update requests together
// Merge behaves similarly to a list append; usage should in the form `a = a.merge(b)`.
// Importantly, Merge may decide to allocate a new PushRequest object or reuse the existing one - both
// inputs should not be used after completion.
func (pr *PushRequest) Merge(other *PushRequest) *PushRequest {
	if pr == nil {
		return other
	}
	if other == nil {
		return pr
	}

	// Keep the first (older) start time

	// Merge the two reasons. Note that we shouldn't deduplicate here, or we would under count
	if len(other.Reason) > 0 {
		if pr.Reason == nil {
			pr.Reason = make(map[TriggerReason]int)
		}
		pr.Reason.Merge(other.Reason)
	}

	// If either is full we need a full push
	pr.Full = pr.Full || other.Full

	// The other push context is presumed to be later and more up to date
	if other.Push != nil {
		pr.Push = other.Push
	}

	// Do not merge when any one is empty
	if len(pr.ConfigsUpdated) == 0 || len(other.ConfigsUpdated) == 0 {
		pr.ConfigsUpdated = nil
	} else {
		for conf := range other.ConfigsUpdated {
			pr.ConfigsUpdated.Insert(conf)
		}
	}

	return pr
}

// CopyMerge two update requests together. Unlike Merge, this will not mutate either input.
// This should be used when we are modifying a shared PushRequest (typically any time it's in the context
// of a single proxy)
func (pr *PushRequest) CopyMerge(other *PushRequest) *PushRequest {
	if pr == nil {
		return other
	}
	if other == nil {
		return pr
	}

	var reason ReasonStats
	if len(pr.Reason)+len(other.Reason) > 0 {
		reason = make(ReasonStats)
		reason.Merge(pr.Reason)
		reason.Merge(other.Reason)
	}
	merged := &PushRequest{
		// Keep the first (older) start time
		Start: pr.Start,

		// If either is full we need a full push
		Full: pr.Full || other.Full,

		// The other push context is presumed to be later and more up to date
		Push: other.Push,

		// Merge the two reasons. Note that we shouldn't deduplicate here, or we would under count
		Reason: reason,
	}

	// Do not merge when any one is empty
	if len(pr.ConfigsUpdated) > 0 && len(other.ConfigsUpdated) > 0 {
		merged.ConfigsUpdated = make(sets.Set[ConfigKey], len(pr.ConfigsUpdated)+len(other.ConfigsUpdated))
		merged.ConfigsUpdated.Merge(pr.ConfigsUpdated)
		merged.ConfigsUpdated.Merge(other.ConfigsUpdated)
	}

	return merged
}

func (pr *PushRequest) IsRequest() bool {
	return len(pr.Reason) == 1 && pr.Reason.Has(ProxyRequest)
}

func (pr *PushRequest) IsProxyUpdate() bool {
	return pr.Reason.Has(ProxyUpdate)
}

func (pr *PushRequest) PushReason() string {
	if pr.IsRequest() {
		return " request"
	}
	return ""
}
