// Copyright (C) 2021 Cisco Systems Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cni

import (
	"github.com/pkg/errors"

	"github.com/projectcalico/vpp-dataplane/v3/calico-vpp-agent/cni/storage"
	"github.com/projectcalico/vpp-dataplane/v3/calico-vpp-agent/common"
	"github.com/projectcalico/vpp-dataplane/v3/calico-vpp-agent/watchers"
	"github.com/projectcalico/vpp-dataplane/v3/vpplink"
	"github.com/projectcalico/vpp-dataplane/v3/vpplink/types"
)

func (s *Server) RoutePodInterface(podSpec *storage.LocalPodSpec, stack *vpplink.CleanupStack, swIfIndex uint32, isL3 bool, inPodVrf bool) error {
	for _, containerIP := range podSpec.GetContainerIps() {
		var table uint32
		if podSpec.NetworkName != "" {
			idx := 0
			if vpplink.IsIP6(containerIP.IP) {
				idx = 1
			}
			value, ok := s.networkDefinitions.Load(podSpec.NetworkName)
			if !ok {
				s.log.Errorf("network not found %s", podSpec.NetworkName)
			} else {
				networkDefinition, ok := value.(*watchers.NetworkDefinition)
				if !ok || networkDefinition == nil {
					panic("networkDefinition not of type *watchers.NetworkDefinition")
				}
				table = networkDefinition.VRF.Tables[idx]
			}
		} else if inPodVrf {
			table = podSpec.GetVrfID(vpplink.IPFamilyFromIPNet(containerIP))
		}
		route := types.Route{
			Dst: containerIP,
			Paths: []types.RoutePath{{
				SwIfIndex: swIfIndex,
			}},
			Table: table,
		}
		s.log.Infof("pod(add) route [podVRF ->MainIF] %s", route.String())
		err := s.vpp.RouteAdd(&route)
		if err != nil {
			return errors.Wrapf(err, "Cannot adding route [podVRF ->MainIF] %s", route.String())
		} else {
			stack.Push(s.vpp.RouteDel, &route)
		}
		if !isL3 {
			s.log.Infof("pod(add) neighbor if[%d] %s", swIfIndex, containerIP.IP.String())
			err = s.vpp.AddNeighbor(&types.Neighbor{
				SwIfIndex:    swIfIndex,
				IP:           containerIP.IP,
				HardwareAddr: common.ContainerSideMacAddress,
				Flags:        types.IPNeighborStatic,
			})
			if err != nil {
				return errors.Wrapf(err, "Error adding neighbor if[%d] %s", swIfIndex, containerIP.IP.String())
			}
		}
	}
	return nil
}

func (s *Server) UnroutePodInterface(podSpec *storage.LocalPodSpec, swIfIndex uint32, inPodVrf bool) {
	for _, containerIP := range podSpec.GetContainerIps() {
		var table uint32
		if podSpec.NetworkName != "" {
			idx := 0
			if vpplink.IsIP6(containerIP.IP) {
				idx = 1
			}
			value, ok := s.networkDefinitions.Load(podSpec.NetworkName)
			if !ok {
				s.log.Errorf("network not found %s", podSpec.NetworkName)
			} else {
				networkDefinition, ok := value.(*watchers.NetworkDefinition)
				if !ok || networkDefinition == nil {
					panic("networkDefinition not of type *watchers.NetworkDefinition")
				}
				table = networkDefinition.VRF.Tables[idx]
			}
		} else if inPodVrf {
			table = podSpec.GetVrfID(vpplink.IPFamilyFromIPNet(containerIP))
		}
		route := types.Route{
			Dst: containerIP,
			Paths: []types.RoutePath{{
				SwIfIndex: swIfIndex,
			}},
			Table: table,
		}
		s.log.Infof("pod(del) route [podVRF ->MainIF] %s", route.String())
		err := s.vpp.RouteDel(&route)
		if err != nil {
			s.log.Warnf("Error deleting route [podVRF ->MainIF] %s : %s", route.String(), err)
		}
	}
}

func (s *Server) RoutePblPortsPodInterface(podSpec *storage.LocalPodSpec, stack *vpplink.CleanupStack, swIfIndex uint32, isL3 bool) (err error) {
	for _, containerIP := range podSpec.GetContainerIps() {
		path := types.RoutePath{
			SwIfIndex: swIfIndex,
		}
		if !isL3 {
			path.Gw = containerIP.IP
		}

		portRanges := make([]types.PblPortRange, 0)
		for _, pc := range podSpec.IfPortConfigs {
			portRanges = append(portRanges, types.PblPortRange{
				Start: pc.Start,
				End:   pc.End,
				Proto: pc.Proto,
			})
		}

		// See docs/_static/calico_vpp_vrf_layout.drawio
		client := types.PblClient{
			ID:         vpplink.InvalidID,
			TableID:    podSpec.GetVrfID(vpplink.IPFamilyFromIPNet(containerIP)),
			Addr:       containerIP.IP,
			Path:       path,
			PortRanges: portRanges,
		}
		if podSpec.EnableVCL {
			client.TableID = common.PuntTableID
		}

		vrfID := podSpec.GetVrfID(vpplink.IPFamilyFromIPNet(containerIP)) // pbl only supports v4 ?
		s.log.Infof("pod(add) PBL client for %s VRF %d", containerIP.IP, vrfID)
		pblIndex, err := s.vpp.AddPblClient(&client)
		if err != nil {
			return errors.Wrapf(err, "error adding PBL client for %s VRF %d", containerIP.IP, vrfID)
		} else {
			stack.Push(s.vpp.DelPblClient, pblIndex)
		}
		podSpec.PblIndex = pblIndex

		if !isL3 {
			s.log.Infof("pod(add) neighbor if[%d] %s", swIfIndex, containerIP.IP.String())
			err = s.vpp.AddNeighbor(&types.Neighbor{
				SwIfIndex:    swIfIndex,
				IP:           containerIP.IP,
				HardwareAddr: common.ContainerSideMacAddress,
				Flags:        types.IPNeighborStatic,
			})
			if err != nil {
				return errors.Wrapf(err, "Cannot add neighbor if[%d] %s", swIfIndex, containerIP.IP.String())
			}
		}
	}
	return nil
}

func (s *Server) UnroutePblPortsPodInterface(podSpec *storage.LocalPodSpec, swIfIndex uint32) {
	s.log.Infof("pod(del) PBL client[%d]", podSpec.PblIndex)
	err := s.vpp.DelPblClient(podSpec.PblIndex)
	if err != nil {
		s.log.Warnf("Error deleting pbl conf %s", err)
	}
}

func (s *Server) CreatePodRPFVRF(podSpec *storage.LocalPodSpec, stack *vpplink.CleanupStack) (err error) {
	for _, ipFamily := range vpplink.IPFamilies {
		vrfID, err := s.vpp.AllocateVRF(ipFamily.IsIP6, podSpec.GetVrfTag(ipFamily, "RPF"))
		podSpec.SetRPFVrfID(vrfID, ipFamily)
		s.log.Debugf("Allocated %s RPFVRF ID:%d", ipFamily.Str, vrfID)
		if err != nil {
			return errors.Wrapf(err, "error allocating VRF %s", ipFamily.Str)
		} else {
			stack.Push(s.vpp.DelVRF, vrfID, ipFamily.IsIP6)
		}
	}
	return nil
}

func (s *Server) CreatePodVRF(podSpec *storage.LocalPodSpec, stack *vpplink.CleanupStack) (err error) {
	/* Create and Setup the per-pod VRF */
	for _, ipFamily := range vpplink.IPFamilies {
		vrfID, err := s.vpp.AllocateVRF(ipFamily.IsIP6, podSpec.GetVrfTag(ipFamily, ""))
		podSpec.SetVrfID(vrfID, ipFamily)
		s.log.Debugf("Allocated %s VRF ID:%d", ipFamily.Str, vrfID)
		if err != nil {
			return errors.Wrapf(err, "error allocating VRF %s", ipFamily.Str)
		} else {
			stack.Push(s.vpp.DelVRF, vrfID, ipFamily.IsIP6)
		}
	}

	for idx, ipFamily := range vpplink.IPFamilies {
		vrfID := podSpec.GetVrfID(ipFamily)
		var vrfIndex uint32
		if podSpec.NetworkName == "" { // no multi net
			vrfIndex = common.PodVRFIndex
		} else {
			value, ok := s.networkDefinitions.Load(podSpec.NetworkName)
			if !ok {
				return errors.Errorf("network not found %s", podSpec.NetworkName)
			}
			networkDefinition, ok := value.(*watchers.NetworkDefinition)
			if !ok || networkDefinition == nil {
				panic("networkDefinition not of type *watchers.NetworkDefinition")
			}
			vrfIndex = networkDefinition.PodVRF.Tables[idx]
		}
		s.log.Infof("pod(add) VRF %d %s default route via VRF %d", vrfID, ipFamily.Str, vrfIndex)
		err = s.vpp.AddDefaultRouteViaTable(vrfID, vrfIndex, ipFamily.IsIP6)
		if err != nil {
			return errors.Wrapf(err, "error adding VRF %d %s default route via VRF %d", vrfID, ipFamily.Str, vrfIndex)
		} else {
			stack.Push(s.vpp.DelDefaultRouteViaTable, vrfID, vrfIndex, ipFamily.IsIP6)
		}

		err = s.vpp.AddDefaultMRouteViaTable(vrfID, vrfIndex, ipFamily.IsIP6)
		if err != nil {
			return errors.Wrapf(err, "error adding VRF %d %s default Mroute via VRF %d", vrfID, ipFamily.Str, vrfIndex)
		} else {
			stack.Push(s.vpp.DelDefaultMRouteViaTable, vrfID, vrfIndex, ipFamily.IsIP6)
		}
	}
	return nil
}

func (s *Server) ActivateStrictRPF(podSpec *storage.LocalPodSpec, stack *vpplink.CleanupStack) (err error) {
	s.log.Infof("pod(add) create pod RPF VRF")
	err = s.CreatePodRPFVRF(podSpec, stack)
	if err != nil {
		return errors.Wrapf(err, "failed to create RPFVrf for pod")
	}
	s.log.Infof("pod(add) add routes for RPF VRF")
	err = s.AddRPFRoutes(podSpec, stack)
	if err != nil {
		return errors.Wrapf(err, "failed to add routes for RPF VRF")
	}
	s.log.Infof("pod(add) set custom-vrf urpf")
	err = s.vpp.SetCustomURPF(podSpec.TunTapSwIfIndex, podSpec.V4RPFVrfID)
	if err != nil {
		return errors.Wrapf(err, "failed to set urpf strict on interface")
	} else {
		stack.Push(s.vpp.UnsetURPF, podSpec.TunTapSwIfIndex)
	}
	return nil
}

func (s *Server) AddRPFRoutes(podSpec *storage.LocalPodSpec, stack *vpplink.CleanupStack) (err error) {
	for _, containerIP := range podSpec.GetContainerIps() {
		RPFvrfID := podSpec.GetRPFVrfID(vpplink.IPFamilyFromIPNet(containerIP))
		// Always there (except multinet memif)
		pathsToPod := []types.RoutePath{{
			SwIfIndex: podSpec.TunTapSwIfIndex,
			Gw:        containerIP.IP,
		}}
		// Add pbl memif case
		if podSpec.MemifSwIfIndex != vpplink.InvalidSwIfIndex {
			pathsToPod = append(pathsToPod, types.RoutePath{
				SwIfIndex: podSpec.MemifSwIfIndex,
				Gw:        containerIP.IP,
			})
			s.log.Infof("pod(add) add route to %+v in rpfvrf %+v via memif and tun", podSpec.GetContainerIps(), RPFvrfID)
		} else {
			s.log.Infof("pod(add) add route to %+v in rpfvrf %+v via tun", podSpec.GetContainerIps(), RPFvrfID)
		}
		route := &types.Route{
			Dst:   containerIP,
			Paths: pathsToPod,
			Table: RPFvrfID,
		}
		err = s.vpp.RouteAdd(route)
		if err != nil {
			return errors.Wrapf(err, "error adding RPFVRF %d proper route", RPFvrfID)
		} else {
			stack.Push(s.vpp.RouteDel, route)
		}

		// Add addresses allowed to be spooofed
		if podSpec.AllowedSpoofingPrefixes != "" {
			// Parse Annotation data
			allowedSources, err := s.ParseSpoofAddressAnnotation(podSpec.AllowedSpoofingPrefixes)
			if err != nil {
				return errors.Wrapf(err, "error parsing allowSpoofing addresses")
			}
			for _, allowedSource := range allowedSources {
				s.log.Infof("pod(add) add route to %+v in rpfvrf %+v to allow spoofing", allowedSource.IPNet, RPFvrfID)
				route := &types.Route{
					Dst:   &allowedSource.IPNet,
					Paths: pathsToPod,
					Table: RPFvrfID,
				}
				err = s.vpp.RouteAdd(route)
				if err != nil {
					return errors.Wrapf(err, "error adding RPFVRF %d proper route", RPFvrfID)
				} else {
					stack.Push(s.vpp.RouteDel, route)
				}
			}
		}
	}
	return nil
}

func (s *Server) DeactivateStrictRPF(podSpec *storage.LocalPodSpec) {
	var err error
	for _, containerIP := range podSpec.GetContainerIps() {
		RPFvrfID := podSpec.GetRPFVrfID(vpplink.IPFamilyFromIPNet(containerIP))
		// Always there (except multinet memif)
		pathsToPod := []types.RoutePath{{
			SwIfIndex: podSpec.TunTapSwIfIndex,
			Gw:        containerIP.IP,
		}}
		// pbl memif case
		if podSpec.MemifSwIfIndex != vpplink.InvalidSwIfIndex {
			pathsToPod = append(pathsToPod, types.RoutePath{
				SwIfIndex: podSpec.MemifSwIfIndex,
				Gw:        containerIP.IP,
			})
			s.log.Infof("pod(del) del route to %+v in rpfvrf %+v via memif and tun", podSpec.GetContainerIps(), RPFvrfID)
		} else {
			s.log.Infof("pod(del) del route to %+v in rpfvrf %+v via tun", podSpec.GetContainerIps(), RPFvrfID)
		}
		err = s.vpp.RouteDel(&types.Route{
			Dst:   containerIP,
			Paths: pathsToPod,
			Table: RPFvrfID,
		})
		if err != nil {
			s.log.Errorf("error deleting RPFVRF %d route : %s", RPFvrfID, err)
		}

		// Delete addresses allowed to be spooofed
		if podSpec.AllowedSpoofingPrefixes != "" {
			// Parse Annotation data
			allowedSources, err := s.ParseSpoofAddressAnnotation(podSpec.AllowedSpoofingPrefixes)
			if err != nil {
				s.log.WithError(err).Error("error parsing allowSpoofing addresses")
			}
			for _, allowedSource := range allowedSources {
				s.log.Infof("pod(del) del route to %+v in rpfvrf %+v used to allow spoofing", allowedSource.IPNet, RPFvrfID)
				err = s.vpp.RouteDel(&types.Route{
					Dst:   &allowedSource.IPNet,
					Paths: pathsToPod,
					Table: RPFvrfID,
				})
				if err != nil {
					s.log.Errorf("error deleting VRF %d route: %s", RPFvrfID, err)
				}
			}
		}
	}

	for _, ipFamily := range vpplink.IPFamilies {
		rpfvrfID := podSpec.GetRPFVrfID(ipFamily)
		s.log.Infof("pod(del) RPF-VRF %d %s", rpfvrfID, ipFamily.Str)
		err = s.vpp.DelVRF(rpfvrfID, ipFamily.IsIP6)
		if err != nil {
			s.log.Errorf("Error deleting RPF-VRF %d %s : %s", rpfvrfID, ipFamily.Str, err)
		}
	}
}

func (s *Server) DeletePodVRF(podSpec *storage.LocalPodSpec) {
	var err error
	for idx, ipFamily := range vpplink.IPFamilies {
		vrfID := podSpec.GetVrfID(ipFamily)
		var vrfIndex uint32
		if podSpec.NetworkName == "" {
			vrfIndex = common.PodVRFIndex
		} else {
			value, ok := s.networkDefinitions.Load(podSpec.NetworkName)
			if !ok {
				s.log.Errorf("network not found %s", podSpec.NetworkName)
			} else {
				networkDefinition, ok := value.(*watchers.NetworkDefinition)
				if !ok || networkDefinition == nil {
					panic("networkDefinition not of type *watchers.NetworkDefinition")
				}
				vrfIndex = networkDefinition.PodVRF.Tables[idx]
			}
		}
		s.log.Infof("pod(del) VRF %d %s default route via VRF %d", vrfID, ipFamily.Str, vrfIndex)
		err = s.vpp.DelDefaultRouteViaTable(vrfID, vrfIndex, ipFamily.IsIP6)
		if err != nil {
			s.log.Errorf("Error deleting VRF %d %s default route via VRF %d : %s", vrfID, ipFamily.Str, vrfIndex, err)
		}
		err = s.vpp.DelDefaultMRouteViaTable(vrfID, vrfIndex, ipFamily.IsIP6)
		if err != nil {
			s.log.Errorf("Error deleting VRF %d %s default mroute via VRF %d : %s", vrfID, ipFamily.Str, vrfIndex, err)
		}
	}

	for _, ipFamily := range vpplink.IPFamilies {
		vrfID := podSpec.GetVrfID(ipFamily)
		s.log.Infof("pod(del) VRF %d %s", vrfID, ipFamily.Str)
		err = s.vpp.DelVRF(vrfID, ipFamily.IsIP6)
		if err != nil {
			s.log.Errorf("Error deleting VRF %d %s : %s", vrfID, ipFamily.Str, err)
		}
	}
}

func (s *Server) CreateVRFRoutesToPod(podSpec *storage.LocalPodSpec, stack *vpplink.CleanupStack) (err error) {
	for _, containerIP := range podSpec.GetContainerIps() {
		/* In the main table route the container address to its VRF */
		route := types.Route{
			Dst: containerIP,
			Paths: []types.RoutePath{{
				Table:     podSpec.GetVrfID(vpplink.IPFamilyFromIPNet(containerIP)),
				SwIfIndex: types.InvalidID,
			}},
		}
		s.log.Infof("pod(add) route [mainVRF->PodVRF] %s", route.String())
		err := s.vpp.RouteAdd(&route)
		if err != nil {
			return errors.Wrapf(err, "error adding route [mainVRF ->PodVRF] %s", route.String())
		} else {
			stack.Push(s.vpp.RouteDel, &route)
		}
	}
	return nil
}

func (s *Server) DeleteVRFRoutesToPod(podSpec *storage.LocalPodSpec) {
	for _, containerIP := range podSpec.GetContainerIps() {
		/* In the main table route the container address to its VRF */
		route := types.Route{
			Dst: containerIP,
			Paths: []types.RoutePath{{
				Table:     podSpec.GetVrfID(vpplink.IPFamilyFromIPNet(containerIP)),
				SwIfIndex: types.InvalidID,
			}},
		}
		s.log.Infof("pod(del) route [mainVRF->PodVRF] %s", route.String())
		err := s.vpp.RouteDel(&route)
		if err != nil {
			s.log.Errorf("error deleting vpp side routes route [mainVRF ->PodVRF] %s : %s", route.String(), err)
		}
	}
}

func (s *Server) SetupPuntRoutes(podSpec *storage.LocalPodSpec, stack *vpplink.CleanupStack, swIfIndex uint32) (err error) {
	for _, containerIP := range podSpec.GetContainerIps() {
		/* In the punt table (where all punted traffics ends),
		 * route the container to the tun */
		route := types.Route{
			Table: common.PuntTableID,
			Dst:   containerIP,
			Paths: []types.RoutePath{{SwIfIndex: swIfIndex}},
		}
		s.log.Infof("pod(add) route [puntVRF->PuntIF] %s", route.String())
		err = s.vpp.RouteAdd(&route)
		if err != nil {
			return errors.Wrapf(err, "error adding vpp side routes for interface")
		} else {
			stack.Push(s.vpp.RouteDel, &route)
		}
	}
	return nil
}

func (s *Server) RemovePuntRoutes(podSpec *storage.LocalPodSpec, swIfIndex uint32) {
	for _, containerIP := range podSpec.GetContainerIps() {
		/* In the punt table (where all punted traffics ends), route the container to the tun */
		route := types.Route{
			Table: common.PuntTableID,
			Dst:   containerIP,
			Paths: []types.RoutePath{{SwIfIndex: swIfIndex}},
		}
		s.log.Infof("pod(del) route [puntVRF->PuntIF] %s", route.String())
		err := s.vpp.RouteDel(&route)
		if err != nil {
			s.log.Errorf("error deleting route [puntVRF ->PuntIF] %s : %s", route.String(), err)
		}
	}
}
