/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kube2store

import (
	discovery "k8s.io/api/discovery/v1beta1"
	"k8s.io/klog"

	localnetv12 "sigs.k8s.io/kpng/api/localnetv1"
	proxystore2 "sigs.k8s.io/kpng/server/pkg/proxystore"
)

const hostNameLabel = "kubernetes.io/hostname"

type sliceEventHandler struct{ eventHandler }

func serviceNameFrom(eps *discovery.EndpointSlice) string {
	if eps.Labels == nil {
		return ""
	}
	return eps.Labels[discovery.LabelServiceName]
}

func (h sliceEventHandler) OnAdd(obj interface{}) {
	eps := obj.(*discovery.EndpointSlice)

	serviceName := serviceNameFrom(eps)
	if serviceName == "" {
		// no name => not associated with a service => ignore
		return
	}

	// compute endpoints
	infos := make([]*localnetv12.EndpointInfo, 0, len(eps.Endpoints))

	for _, sliceEndpoint := range eps.Endpoints {
		info := &localnetv12.EndpointInfo{
			Namespace:   eps.Namespace,
			ServiceName: serviceName,
			SourceName:  eps.Name,
			Topology:    sliceEndpoint.Topology,
			Endpoint:    &localnetv12.Endpoint{},
			Conditions:  &localnetv12.EndpointConditions{},
		}

		if sliceEndpoint.Topology != nil {
			info.NodeName = sliceEndpoint.Topology[hostNameLabel]
		}

		if h := sliceEndpoint.Hostname; h != nil {
			info.Endpoint.Hostname = *h
		}

		if r := sliceEndpoint.Conditions.Ready; r != nil && *r {
			info.Conditions.Ready = true
		}

		for _, addr := range sliceEndpoint.Addresses {
			info.Endpoint.AddAddress(addr)
		}

		infos = append(infos, info)
	}

	h.s.Update(func(tx *proxystore2.Tx) {
		tx.SetEndpointsOfSource(eps.Namespace, eps.Name, infos)
		h.updateSync(proxystore2.Endpoints, tx)

		if log := klog.V(3); log {
			log.Info("endpoints of ", eps.Namespace, "/", serviceName, ":")
			tx.EachEndpointOfService(eps.Namespace, serviceName, func(ei *localnetv12.EndpointInfo) {
				log.Info("- ", ei.Endpoint.IPs, " | topo: ", ei.Topology)
			})
		}
	})
}

func (h sliceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	// same as adding
	h.OnAdd(newObj)
}

func (h sliceEventHandler) OnDelete(oldObj interface{}) {
	eps := oldObj.(*discovery.EndpointSlice)

	h.s.Update(func(tx *proxystore2.Tx) {
		tx.DelEndpointsOfSource(eps.Namespace, eps.Name)
		h.updateSync(proxystore2.Endpoints, tx)
	})
}
