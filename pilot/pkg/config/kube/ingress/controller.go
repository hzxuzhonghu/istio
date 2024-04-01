// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package ingress provides a read-only view of Kubernetes ingress resources
// as an ingress rule configuration type store
package ingress

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	corev1 "k8s.io/api/core/v1"
	knetworking "k8s.io/api/networking/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	meshconfig "istio.io/api/mesh/v1alpha1"
	"istio.io/istio/pilot/pkg/model"
	kubecontroller "istio.io/istio/pilot/pkg/serviceregistry/kube/controller"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/mesh"
	"istio.io/istio/pkg/config/schema/collection"
	"istio.io/istio/pkg/config/schema/collections"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/env"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/util/sets"
)

// In 1.0, the Gateway is defined in the namespace where the actual controller runs, and needs to be managed by
// user.
// The gateway is named by appending "-istio-autogenerated-k8s-ingress" to the name of the ingress.
//
// Currently the gateway namespace is hardcoded to istio-system (model.IstioIngressNamespace)
//
// VirtualServices are also auto-generated in the model.IstioIngressNamespace.
//
// The sync of Ingress objects to IP is done by status.go
// the 'ingress service' name is used to get the IP of the Service
// If ingress service is empty, it falls back to NodeExternalIP list, selected using the labels.
// This is using 'namespace' of pilot - but seems to be broken (never worked), since it uses Pilot's pod labels
// instead of the ingress labels.

// Follows mesh.IngressControllerMode setting to enable - OFF|STRICT|DEFAULT.
// STRICT requires "kubernetes.io/ingress.class" == mesh.IngressClass
// DEFAULT allows Ingress without explicit class.

// In 1.1:
// - K8S_INGRESS_NS - namespace of the Gateway that will act as ingress.
// - labels of the gateway set to "app=ingressgateway" for node_port, service set to 'ingressgateway' (matching default install)
//   If we need more flexibility - we can add it (but likely we'll deprecate ingress support first)
// -

var schemas = collection.SchemasFor(
	collections.VirtualService,
	collections.Gateway)

// Control needs RBAC permissions to write to Pods.

type controller struct {
	meshWatcher  mesh.Holder
	domainSuffix string

	queue                  controllers.Queue
	virtualServiceHandlers []model.EventHandler
	gatewayHandlers        []model.EventHandler

	mutex sync.RWMutex
	// processed ingresses
	ingresses map[types.NamespacedName]*knetworking.Ingress

	classes  kclient.Client[*knetworking.IngressClass]
	ingress  kclient.Client[*knetworking.Ingress]
	services kclient.Client[*corev1.Service]
}

var IngressNamespace = env.Register("K8S_INGRESS_NS", constants.IstioIngressNamespace, "").Get()

var errUnsupportedOp = errors.New("unsupported operation: the ingress config store is a read-only view")

// NewController creates a new Kubernetes controller
func NewController(client kube.Client, meshWatcher mesh.Holder,
	options kubecontroller.Options,
) model.ConfigStoreController {
	ingress := kclient.NewFiltered[*knetworking.Ingress](client, kclient.Filter{ObjectFilter: client.ObjectFilter()})
	classes := kclient.New[*knetworking.IngressClass](client)
	services := kclient.NewFiltered[*corev1.Service](client, kclient.Filter{ObjectFilter: client.ObjectFilter()})

	c := &controller{
		meshWatcher:  meshWatcher,
		domainSuffix: options.DomainSuffix,
		ingresses:    make(map[types.NamespacedName]*knetworking.Ingress),
		ingress:      ingress,
		classes:      classes,
		services:     services,
	}
	c.queue = controllers.NewQueue("ingress",
		controllers.WithReconciler(c.onEvent),
		controllers.WithMaxAttempts(5))
	c.ingress.AddEventHandler(controllers.ObjectHandler(c.queue.AddObject))

	// We watch service changes to detect service port number change to trigger
	// re-convert ingress to new-vs.
	c.services.AddEventHandler(controllers.FromEventHandler(func(o controllers.Event) {
		c.onServiceEvent(o)
	}))

	return c
}

func (c *controller) Run(stop <-chan struct{}) {
	kube.WaitForCacheSync("ingress", stop, c.ingress.HasSynced, c.services.HasSynced, c.classes.HasSynced)
	c.queue.Run(stop)
	controllers.ShutdownAll(c.ingress, c.services, c.classes)
}

func (c *controller) shouldProcessIngress(mesh *meshconfig.MeshConfig, i *knetworking.Ingress) bool {
	var class *knetworking.IngressClass
	if i.Spec.IngressClassName != nil {
		c := c.classes.Get(*i.Spec.IngressClassName, "")
		if c == nil {
			return false
		}
		class = c
	}
	return shouldProcessIngressWithClass(mesh, i, class)
}

// shouldProcessIngressUpdate checks whether we should renotify registered handlers about an update event
func (c *controller) shouldProcessIngressUpdate(ing *knetworking.Ingress) bool {
	// ingress add/update
	shouldProcess := c.shouldProcessIngress(c.meshWatcher.Mesh(), ing)
	item := config.NamespacedName(ing)
	if shouldProcess {
		// record processed ingress
		c.mutex.Lock()
		c.ingresses[item] = ing
		c.mutex.Unlock()
		return true
	}

	c.mutex.Lock()
	_, preProcessed := c.ingresses[item]
	// previous processed but should not currently, delete it
	if preProcessed && !shouldProcess {
		delete(c.ingresses, item)
	} else {
		c.ingresses[item] = ing
	}
	c.mutex.Unlock()

	return preProcessed
}

func (c *controller) onEvent(item types.NamespacedName) error {
	event := model.EventUpdate
	ing := c.ingress.Get(item.Name, item.Namespace)
	if ing == nil {
		event = model.EventDelete
		c.mutex.Lock()
		ing = c.ingresses[item]
		delete(c.ingresses, item)
		c.mutex.Unlock()
		if ing == nil {
			// It was a delete and we didn't have an existing known ingress, no action
			return nil
		}
	}

	// we should check need process only when event is not delete,
	// if it is delete event, and previously processed, we need to process too.
	if event != model.EventDelete {
		shouldProcess := c.shouldProcessIngressUpdate(ing)
		if !shouldProcess {
			return nil
		}
	}

	vsmetadata := config.Meta{
		Name:             item.Name + "-" + "virtualservice",
		Namespace:        item.Namespace,
		GroupVersionKind: gvk.VirtualService,
	}
	gatewaymetadata := config.Meta{
		Name:             item.Name + "-" + "gateway",
		Namespace:        item.Namespace,
		GroupVersionKind: gvk.Gateway,
	}

	// Trigger updates for Gateway and VirtualService
	// TODO: we could be smarter here and only trigger when real changes were found
	for _, f := range c.virtualServiceHandlers {
		f(config.Config{Meta: vsmetadata}, config.Config{Meta: vsmetadata}, event)
	}
	for _, f := range c.gatewayHandlers {
		f(config.Config{Meta: gatewaymetadata}, config.Config{Meta: gatewaymetadata}, event)
	}

	return nil
}

func (c *controller) onServiceEvent(input any) {
	event := input.(controllers.Event)
	curSvc := event.Latest().(*corev1.Service)

	// This is shortcut. We only care about the port number change if we receive service update event.
	if event.Event == controllers.EventUpdate {
		oldSvc := event.Old.(*corev1.Service)
		oldPorts := extractPorts(oldSvc.Spec.Ports)
		curPorts := extractPorts(curSvc.Spec.Ports)
		// If the ports don't change, we do nothing.
		if oldPorts.Equals(curPorts) {
			return
		}
	}

	// We care about add, delete and ports changed event of services that are referred
	// by ingress using port name.
	namespacedName := config.NamespacedName(curSvc).String()
	for _, ingress := range c.ingress.List(curSvc.GetNamespace(), klabels.Everything()) {
		referredSvcSet := extractServicesByPortNameType(ingress)
		if referredSvcSet.Contains(namespacedName) {
			c.queue.AddObject(ingress)
		}
	}
}

func (c *controller) RegisterEventHandler(kind config.GroupVersionKind, f model.EventHandler) {
	switch kind {
	case gvk.VirtualService:
		c.virtualServiceHandlers = append(c.virtualServiceHandlers, f)
	case gvk.Gateway:
		c.gatewayHandlers = append(c.gatewayHandlers, f)
	}
}

func (c *controller) HasSynced() bool {
	return c.queue.HasSynced()
}

func (c *controller) Schemas() collection.Schemas {
	// TODO: are these two config descriptors right?
	return schemas
}

func (c *controller) Get(typ config.GroupVersionKind, name, namespace string) *config.Config {
	return nil
}

// sortIngressByCreationTime sorts the list of config objects in ascending order by their creation time (if available).
func sortIngressByCreationTime(ingr []*knetworking.Ingress) []*knetworking.Ingress {
	sort.Slice(ingr, func(i, j int) bool {
		// If creation time is the same, then behavior is nondeterministic. In this case, we can
		// pick an arbitrary but consistent ordering based on name and namespace, which is unique.
		// CreationTimestamp is stored in seconds, so this is not uncommon.
		if ingr[i].CreationTimestamp == ingr[j].CreationTimestamp {
			in := ingr[i].Name + "." + ingr[i].Namespace
			jn := ingr[j].Name + "." + ingr[j].Namespace
			return in < jn
		}
		return ingr[i].CreationTimestamp.Before(&ingr[j].CreationTimestamp)
	})
	return ingr
}

func (c *controller) List(typ config.GroupVersionKind, namespace string) []config.Config {
	if typ != gvk.Gateway &&
		typ != gvk.VirtualService {
		return nil
	}

	out := make([]config.Config, 0)
	ingressByHost := map[string]*config.Config{}
	for _, ingress := range sortIngressByCreationTime(c.ingress.List(namespace, klabels.Everything())) {
		process := c.shouldProcessIngress(c.meshWatcher.Mesh(), ingress)
		if !process {
			continue
		}

		switch typ {
		case gvk.VirtualService:
			ConvertIngressVirtualService(*ingress, c.domainSuffix, ingressByHost, c.services)
		case gvk.Gateway:
			gateways := ConvertIngressV1alpha3(*ingress, c.meshWatcher.Mesh(), c.domainSuffix)
			out = append(out, gateways)
		}
	}

	if typ == gvk.VirtualService {
		for _, obj := range ingressByHost {
			out = append(out, *obj)
		}
	}

	return out
}

// extractServicesByPortNameType extract services that are of port name type in the specified ingress resource.
func extractServicesByPortNameType(ingress *knetworking.Ingress) sets.String {
	services := sets.String{}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}

		for _, route := range rule.HTTP.Paths {
			if route.Backend.Service == nil {
				continue
			}

			if route.Backend.Service.Port.Name != "" {
				services.Insert(types.NamespacedName{
					Namespace: ingress.GetNamespace(),
					Name:      route.Backend.Service.Name,
				}.String())
			}
		}
	}
	return services
}

func extractPorts(ports []corev1.ServicePort) sets.String {
	result := sets.String{}
	for _, port := range ports {
		// the format is port number|port name.
		result.Insert(fmt.Sprintf("%d|%s", port.Port, port.Name))
	}
	return result
}

func (c *controller) Create(_ config.Config) (string, error) {
	return "", errUnsupportedOp
}

func (c *controller) Update(_ config.Config) (string, error) {
	return "", errUnsupportedOp
}

func (c *controller) UpdateStatus(config.Config) (string, error) {
	return "", errUnsupportedOp
}

func (c *controller) Patch(_ config.Config, _ config.PatchFunc) (string, error) {
	return "", errUnsupportedOp
}

func (c *controller) Delete(_ config.GroupVersionKind, _, _ string, _ *string) error {
	return errUnsupportedOp
}
