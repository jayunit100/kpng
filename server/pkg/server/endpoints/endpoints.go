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

package endpoints

import (
	"google.golang.org/grpc"

	localnetv12 "sigs.k8s.io/kpng/api/localnetv1"
	proxystore2 "sigs.k8s.io/kpng/server/pkg/proxystore"
)

func Setup(s grpc.ServiceRegistrar, store *proxystore2.Store) {
	localnetv12.RegisterEndpointsService(s, localnetv12.NewEndpointsService(localnetv12.UnstableEndpointsService(&Server{
		Store: store,
	})))
}
