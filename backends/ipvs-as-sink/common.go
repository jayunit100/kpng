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

package ipvssink

import (
	"net"
	"sigs.k8s.io/kpng/server/backends/util/ipvs"
	localnetv12 "sigs.k8s.io/kpng/server/pkg/api/localnetv1"
	"strings"

	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"
	netutils "k8s.io/utils/net"
)

func (s *Backend) AddOrDelEndPointInIPSet(endPointList []string, portList []*localnetv12.PortMapping, op Operation) {
	//if !destination.isLocalEndPoint {
	//	return
	//}
	for _, port := range portList {
		for _, endPointIP := range endPointList {
			epIPFamily := getIPFamily(endPointIP)
			ipSetName := loopBackIPSetMap[epIPFamily]
			entry := getEndPointEntry(endPointIP, port)
			if valid := s.ipsetList[ipSetName].validateEntry(entry); !valid {
				klog.Errorf("error adding entry to ipset. entry:%s, ipset:%s", entry.String(), s.ipsetList[ipSetName].Name)
				return
			}
			if op == AddEndPoint {
				s.ipsetList[ipSetName].newEntries.Insert(entry.String())
			}
			if op == DeleteEndPoint {
				s.ipsetList[ipSetName].deleteEntries.Insert(entry.String())
			}
		}
	}
}

func getIPFamily(ipAddr string) v1.IPFamily {
	var ipAddrFamily v1.IPFamily
	if netutils.IsIPv4String(ipAddr) {
		ipAddrFamily = v1.IPv4Protocol
	}

	if netutils.IsIPv6String(ipAddr) {
		ipAddrFamily = v1.IPv6Protocol
	}
	return ipAddrFamily
}

func getEndPointEntry(endPointIP string, port *localnetv12.PortMapping) *ipvs.Entry {
	return &ipvs.Entry{
		IP:       endPointIP,
		Port:     int(port.TargetPort),
		Protocol: strings.ToLower(port.Protocol.String()),
		IP2:      endPointIP,
		SetType:  ipvs.HashIPPortIP,
	}
}

func (s *Backend) AddOrDelClusterIPInIPSet(svc *localnetv12.Service, portList []*localnetv12.PortMapping, op Operation) {
	svcIPFamily := getServiceIPFamily(svc)

	for _, port := range portList {
		for _, ipFamily := range svcIPFamily {
			var clusterIP string
			if ipFamily == v1.IPv4Protocol {
				clusterIP = svc.IPs.ClusterIPs.V4[0]
			}
			if ipFamily == v1.IPv6Protocol {
				clusterIP = svc.IPs.ClusterIPs.V6[0]
			}
			ipSetName := clusterIPSetMap[ipFamily]
			// Capture the clusterIP.
			entry := getIPSetEntry(clusterIP, port)
			// add service Cluster IP:Port to kubeServiceAccess ip set for the purpose of solving hairpin.
			if valid := s.ipsetList[ipSetName].validateEntry(entry); !valid {
				klog.Errorf("error adding entry :%s, to ipset:%s", entry.String(), s.ipsetList[ipSetName].Name)
				return
			}
			if op == AddService {
				s.ipsetList[ipSetName].newEntries.Insert(entry.String())
			}
			if op == DeleteService {
				s.ipsetList[ipSetName].deleteEntries.Insert(entry.String())
			}
		}
	}
}

func getServiceIPFamily(svc *localnetv12.Service) []v1.IPFamily {
	var svcIPFamily []v1.IPFamily
	if len(svc.IPs.ClusterIPs.V4) > 0 && len(svc.IPs.ClusterIPs.V6) == 0 {
		svcIPFamily = append(svcIPFamily, v1.IPv4Protocol)
	}

	if len(svc.IPs.ClusterIPs.V6) > 0 && len(svc.IPs.ClusterIPs.V4) == 0 {
		svcIPFamily = append(svcIPFamily, v1.IPv6Protocol)
	}

	if len(svc.IPs.ClusterIPs.V4) > 0 && len(svc.IPs.ClusterIPs.V6) > 0 {
		svcIPFamily = append(svcIPFamily, v1.IPv4Protocol, v1.IPv6Protocol)
	}
	return svcIPFamily
}

func getIPSetEntry(svcIP string, port *localnetv12.PortMapping) *ipvs.Entry {
	return &ipvs.Entry{
		IP:       svcIP,
		Port:     int(port.Port),
		Protocol: strings.ToLower(port.Protocol.String()),
		SetType:  ipvs.HashIPPort,
	}
}

func getServiceIP(endPointIP string, svc *localnetv12.Service) string {
	var svcIP string
	if netutils.IsIPv4String(endPointIP) {
		svcIP = svc.IPs.ClusterIPs.V4[0]
	}
	if netutils.IsIPv6String(endPointIP) {
		svcIP = svc.IPs.ClusterIPs.V6[0]
	}
	return svcIP
}

func (s *Backend) addServiceIPToKubeIPVSIntf(prevSvc, curr *localnetv12.Service) {
	// sync dummy IPs
	var prevIPs *localnetv12.IPSet
	if prevSvc == nil {
		prevIPs = localnetv12.NewIPSet()
	} else {
		prevIPs = prevSvc.IPs.All()
	}

	currentIPs := curr.IPs.All()

	added, removed := prevIPs.Diff(currentIPs)

	for _, ip := range asDummyIPs(added) {
		if _, ok := s.dummyIPsRefCounts[ip]; !ok {
			// IP is not referenced so we must add it
			klog.V(2).Info("adding dummy IP ", ip)

			_, ipNet, err := net.ParseCIDR(ip)
			if err != nil {
				klog.Fatalf("failed to parse ip/net %q: %v", ip, err)
			}

			if err = netlink.AddrAdd(s.dummy, &netlink.Addr{IPNet: ipNet}); err != nil {
				klog.Error("failed to add dummy IP ", ip, ": ", err)
			}
		}

		s.dummyIPsRefCounts[ip]++
	}

	for _, ip := range asDummyIPs(removed) {
		s.dummyIPsRefCounts[ip]--
	}
}

func (s *Backend) storeLBSvc(portList []*localnetv12.PortMapping, addrList []string, key, svcType string) {
	for _, ip := range addrList {
		prefix := key + "/" + ip + "/"
		for _, port := range portList {
			lbKey := prefix + epPortSuffix(port)
			s.lbs.Set([]byte(lbKey), 0, ipvsLB{IP: ip, ServiceKey: key, Port: port, SchedulingMethod: s.schedulingMethod, ServiceType: svcType})
		}
	}
}

func (s *Backend) deleteLBSvc(portList []*localnetv12.PortMapping, addrList []string, key string) {
	for _, ip := range addrList {
		prefix := key + "/" + ip + "/"
		for _, port := range portList {
			lbKey := prefix + epPortSuffix(port)
			s.lbs.Delete([]byte(lbKey))
		}
	}
}
