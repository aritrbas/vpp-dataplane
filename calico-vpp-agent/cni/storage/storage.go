// Copyright (C) 2020 Cisco Systems Inc.
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

package storage

import (
	"bytes"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/lunixbochs/struc"
	"github.com/pkg/errors"

	"github.com/projectcalico/vpp-dataplane/v3/calico-vpp-agent/common"
	"github.com/projectcalico/vpp-dataplane/v3/config"
	"github.com/projectcalico/vpp-dataplane/v3/vpplink"
	"github.com/projectcalico/vpp-dataplane/v3/vpplink/types"
)

const (
	CniServerStateFileVersion = 9  // Used to ensure compatibility wen we reload data
	MaxAPITagLen              = 63 /* No more than 64 characters in API tags */
	VrfTagHashLen             = 8  /* how many hash charatecters (b64) of the name in tag prefix (useful when trucated) */
)

// XXX: Increment CniServerStateFileVersion when changing this struct
type LocalIPNet struct {
	MaskSize int    `struc:"int8,sizeof=Mask"`
	IP       net.IP `struc:"[16]byte"`
	Mask     net.IPMask
}

// XXX: Increment CniServerStateFileVersion when changing this struct
type LocalIP struct {
	IP net.IP `struc:"[16]byte"`
}

type VppInterfaceType uint8

const (
	VppIfTypeUnknown VppInterfaceType = iota
	VppIfTypeTunTap
	VppIfTypeMemif
	VppIfTypeVCL
)

func (ift VppInterfaceType) String() string {
	switch ift {
	case VppIfTypeUnknown:
		return "Unknown"
	case VppIfTypeTunTap:
		return "TunTap"
	case VppIfTypeMemif:
		return "Memif"
	case VppIfTypeVCL:
		return "VCL"
	default:
		return "Unknown"
	}
}

func (n *LocalIPNet) String() string {
	ipnet := net.IPNet{
		IP:   n.IP,
		Mask: n.Mask,
	}
	return ipnet.String()
}

func (n *LocalIP) String() string {
	return n.IP.String()
}

func (n *LocalIPNet) UpdateSizes() {
	n.MaskSize = len(n.Mask)
}

func (ps *LocalPodSpec) UpdateSizes() {
	ps.RoutesSize = len(ps.Routes)
	ps.ContainerIpsSize = len(ps.ContainerIps)
	ps.InterfaceNameSize = len(ps.InterfaceName)
	ps.NetnsNameSize = len(ps.NetnsName)
	for _, n := range ps.Routes {
		n.UpdateSizes()
	}
}

func (ps *LocalPodSpec) Key() string {
	return fmt.Sprintf("netns:%s,if:%s", ps.NetnsName, ps.InterfaceName)
}

func (ps *LocalPodSpec) String() string {
	lst := ps.ContainerIps
	strLst := make([]string, 0, len(lst))
	for _, e := range lst {
		strLst = append(strLst, e.String())
	}
	return fmt.Sprintf("%s [%s]", ps.Key(), strings.Join(strLst, ", "))
}

func (ps *LocalPodSpec) FullString() string {
	containerIPs := ps.ContainerIps
	containerIPsLst := make([]string, 0, len(containerIPs))
	for _, e := range containerIPs {
		containerIPsLst = append(containerIPsLst, e.String())
	}
	routes := ps.Routes
	routesLst := make([]string, 0, len(routes))
	for _, e := range routes {
		routesLst = append(routesLst, e.String())
	}
	s := fmt.Sprintf("InterfaceName:      %s\n", ps.InterfaceName)
	s += fmt.Sprintf("NetnsName:          %s\n", ps.NetnsName)
	s += fmt.Sprintf("AllowIPForwarding:  %t\n", ps.AllowIPForwarding)
	s += fmt.Sprintf("Routes:             %s\n", strings.Join(routesLst, ", "))
	s += fmt.Sprintf("ContainerIps:       %s\n", strings.Join(containerIPsLst, ", "))
	s += fmt.Sprintf("Mtu:                %d\n", ps.Mtu)
	s += fmt.Sprintf("OrchestratorID:     %s\n", ps.OrchestratorID)
	s += fmt.Sprintf("WorkloadID:         %s\n", ps.WorkloadID)
	s += fmt.Sprintf("EndpointID:         %s\n", ps.EndpointID)
	s += fmt.Sprintf("HostPorts:          %s\n", types.StrableListToString("", ps.HostPorts))
	s += fmt.Sprintf("IfPortConfigs:      %s\n", types.StrableListToString("", ps.IfPortConfigs))
	s += fmt.Sprintf("PortFilteredIfType: %s\n", ps.PortFilteredIfType.String())
	s += fmt.Sprintf("DefaultIfType:      %s\n", ps.DefaultIfType.String())
	s += fmt.Sprintf("EnableVCL:          %t\n", ps.EnableVCL)
	s += fmt.Sprintf("EnableMemif:        %t\n", ps.EnableMemif)
	s += fmt.Sprintf("IsL3:               %t\n", *ps.IfSpec.IsL3)
	s += fmt.Sprintf("MemifSocketID:      %d\n", ps.MemifSocketID)
	s += fmt.Sprintf("TunTapSwIfIndex:    %d\n", ps.TunTapSwIfIndex)
	s += fmt.Sprintf("MemifSwIfIndex:     %d\n", ps.MemifSwIfIndex)
	s += fmt.Sprintf("LoopbackSwIfIndex:  %d\n", ps.LoopbackSwIfIndex)
	s += fmt.Sprintf("PblIndexes:         %d\n", ps.PblIndex)
	s += fmt.Sprintf("V4VrfID:            %d\n", ps.V4VrfID)
	s += fmt.Sprintf("V6VrfID:            %d\n", ps.V6VrfID)
	return s
}

func (ps *LocalPodSpec) GetParamsForIfType(ifType VppInterfaceType) (swIfIndex uint32, isL3 bool) {
	switch ifType {
	case VppIfTypeTunTap:
		return ps.TunTapSwIfIndex, *ps.IfSpec.IsL3
	case VppIfTypeMemif:
		if !*config.GetCalicoVppFeatureGates().MemifEnabled {
			return types.InvalidID, true
		}
		return ps.MemifSwIfIndex, *ps.PBLMemifSpec.IsL3
	default:
		return types.InvalidID, true
	}
}

func (ps *LocalPodSpec) GetBuffersNeeded() uint64 {
	var buffersNeededForThisPod uint64
	buffersNeededForThisPod += ps.IfSpec.GetBuffersNeeded()
	if ps.NetworkName == "" && ps.EnableMemif {
		buffersNeededForThisPod += ps.PBLMemifSpec.GetBuffersNeeded()
	}
	return buffersNeededForThisPod
}

// XXX: Increment CniServerStateFileVersion when changing this struct
type LocalIfPortConfigs struct {
	Start uint16
	End   uint16
	Proto types.IPProto
}

func (pc *LocalIfPortConfigs) String() string {
	return fmt.Sprintf("%s %d-%d", pc.Proto.String(), pc.Start, pc.End)
}

// XXX: Increment CniServerStateFileVersion when changing this struct
type LocalPodSpec struct {
	InterfaceNameSize int `struc:"int16,sizeof=InterfaceName"`
	InterfaceName     string
	NetnsNameSize     int `struc:"int16,sizeof=NetnsName"`
	NetnsName         string
	AllowIPForwarding bool
	RoutesSize        int `struc:"int16,sizeof=Routes"`
	Routes            []LocalIPNet
	ContainerIpsSize  int `struc:"int16,sizeof=ContainerIps"`
	ContainerIps      []LocalIP
	Mtu               int

	// Pod identifiers
	OrchestratorIDSize int `struc:"int16,sizeof=OrchestratorID"`
	OrchestratorID     string
	WorkloadIDSize     int `struc:"int16,sizeof=WorkloadID"`
	WorkloadID         string
	EndpointIDSize     int `struc:"int16,sizeof=EndpointID"`
	EndpointID         string
	// HostPort
	HostPortsSize int `struc:"int16,sizeof=HostPorts"`
	HostPorts     []HostPortBinding

	IfPortConfigsLen int `struc:"int16,sizeof=IfPortConfigs"`
	IfPortConfigs    []LocalIfPortConfigs
	/* This interface type will traffic MATCHING the portConfigs */
	PortFilteredIfType VppInterfaceType
	/* This interface type will traffic not matching portConfigs */
	DefaultIfType VppInterfaceType
	EnableVCL     bool
	EnableMemif   bool

	IfSpec       config.InterfaceSpec
	PBLMemifSpec config.InterfaceSpec

	/**
	 * Below are VPP internal ids, mutable fields in AddVppInterface
	 * We persist them on the disk to avoid rescanning when the agent is restarting.
	 *
	 * We should be careful during state-reconciliation as they might not be
	 * valid anymore. VRF tags should provide this guarantee
	 */
	MemifSocketID     uint32
	TunTapSwIfIndex   uint32
	MemifSwIfIndex    uint32
	LoopbackSwIfIndex uint32
	PblIndex          uint32

	/**
	 * These fields are only a runtime cache, but we also store them
	 * on the disk for debugging purposes.
	 */
	V4VrfID   uint32
	V6VrfID   uint32
	NeedsSnat bool

	/* Multi net */
	NetworkNameSize int `struc:"int16,sizeof=NetworkName"`
	NetworkName     string

	/* rpf check */
	AllowedSpoofingPrefixesSize int `struc:"int16,sizeof=AllowedSpoofingPrefixes"`
	AllowedSpoofingPrefixes     string

	V4RPFVrfID uint32
	V6RPFVrfID uint32
}

func (ps *LocalPodSpec) Copy() LocalPodSpec {
	newPs := *ps

	newPs.Routes = append(make([]LocalIPNet, 0), ps.Routes...)
	newPs.ContainerIps = append(make([]LocalIP, 0), ps.ContainerIps...)
	newPs.HostPorts = append(make([]HostPortBinding, 0), ps.HostPorts...)
	newPs.IfPortConfigs = append(make([]LocalIfPortConfigs, 0), ps.IfPortConfigs...)

	return newPs

}

// XXX: Increment CniServerStateFileVersion when changing this struct
type HostPortBinding struct {
	HostPort      uint16
	HostIP6       net.IP `struc:"[16]byte"`
	HostIP4       net.IP `struc:"[16]byte"`
	ContainerPort uint16
	EntryID       uint32
	Protocol      types.IPProto
}

func (hp *HostPortBinding) String() string {
	s := fmt.Sprintf("%s %s %s:%d", hp.Protocol.String(), hp.HostIP4, hp.HostIP6, hp.HostPort)
	s += fmt.Sprintf(" cport=%d", hp.ContainerPort)
	s += fmt.Sprintf(" id=%d", hp.EntryID)
	return s
}

/* 8 base64 character hash */
func hash(text string) string {
	h := sha512.Sum512([]byte(text))
	return base64.StdEncoding.EncodeToString(h[:])[:VrfTagHashLen]
}

func TruncateStr(text string, size int) string {
	if len(text) > size {
		return text[:size]
	}
	return text
}

func (ps *LocalPodSpec) GetVrfTag(ipFamily vpplink.IPFamily, custom string) string {
	h := hash(fmt.Sprintf("%s%s%s%s", ipFamily.ShortStr, ps.NetnsName, ps.InterfaceName, custom))
	s := fmt.Sprintf("%s-%s-%s%s-%s", h, ipFamily.ShortStr, ps.InterfaceName, custom, filepath.Base(ps.NetnsName))
	return TruncateStr(s, MaxAPITagLen)
}

func (ps *LocalPodSpec) GetInterfaceTag(prefix string) string {
	h := hash(fmt.Sprintf("%s%s%s", prefix, ps.NetnsName, ps.InterfaceName))
	s := fmt.Sprintf("%s-%s-%s", h, ps.InterfaceName, filepath.Base(ps.NetnsName))
	return TruncateStr(s, MaxAPITagLen)
}

func (ps *LocalPodSpec) GetRoutes() (routes []*net.IPNet) {
	routes = make([]*net.IPNet, 0, len(ps.Routes))
	for _, r := range ps.Routes {
		routes = append(routes, &net.IPNet{
			IP:   r.IP,
			Mask: r.Mask,
		})
	}
	return routes
}

func (ps *LocalPodSpec) GetContainerIps() (containerIPs []*net.IPNet) {
	containerIPs = make([]*net.IPNet, 0, len(ps.ContainerIps))
	for _, containerIP := range ps.ContainerIps {
		containerIPs = append(containerIPs, &net.IPNet{
			IP:   containerIP.IP,
			Mask: common.GetMaxCIDRMask(containerIP.IP),
		})
	}
	return containerIPs
}

func (ps *LocalPodSpec) Hasv46() (hasv4 bool, hasv6 bool) {
	hasv4 = false
	hasv6 = false
	for _, containerIP := range ps.ContainerIps {
		if containerIP.IP.To4() == nil {
			hasv6 = true
		} else {
			hasv4 = true
		}
	}
	return hasv4, hasv6
}

func (ps *LocalPodSpec) GetVrfID(ipFamily vpplink.IPFamily) uint32 {
	if ipFamily.IsIP6 {
		return ps.V6VrfID
	} else {
		return ps.V4VrfID
	}
}

func (ps *LocalPodSpec) GetRPFVrfID(ipFamily vpplink.IPFamily) uint32 {
	if ipFamily.IsIP6 {
		return ps.V6RPFVrfID
	} else {
		return ps.V4RPFVrfID
	}
}

func (ps *LocalPodSpec) SetVrfID(id uint32, ipFamily vpplink.IPFamily) {
	if ipFamily.IsIP6 {
		ps.V6VrfID = id
	} else {
		ps.V4VrfID = id
	}
}

func (ps *LocalPodSpec) SetRPFVrfID(id uint32, ipFamily vpplink.IPFamily) {
	if ipFamily.IsIP6 {
		ps.V6RPFVrfID = id
	} else {
		ps.V4RPFVrfID = id
	}
}

type SavedState struct {
	Version    int `struc:"int32"`
	SpecsCount int `struc:"int32,sizeof=Specs"`
	Specs      []LocalPodSpec
}

func PersistCniServerState(podInterfaceMap map[string]LocalPodSpec, fname string) (err error) {
	var buf bytes.Buffer
	tmpFile := fmt.Sprintf("%s~", fname)
	state := &SavedState{
		Version:    CniServerStateFileVersion,
		SpecsCount: len(podInterfaceMap),
		Specs:      make([]LocalPodSpec, 0, len(podInterfaceMap)),
	}
	for _, podSpec := range podInterfaceMap {
		state.Specs = append(state.Specs, podSpec)
	}
	err = struc.Pack(&buf, state)
	if err != nil {
		return errors.Wrap(err, "Error encoding pod data")
	}

	err = os.WriteFile(tmpFile, buf.Bytes(), 0200)
	if err != nil {
		return errors.Wrapf(err, "Error writing file %s", tmpFile)
	}
	err = os.Rename(tmpFile, fname)
	if err != nil {
		return errors.Wrapf(err, "Error moving file %s", tmpFile)
	}
	return nil
}

func LoadCniServerState(fname string) ([]LocalPodSpec, error) {
	var state SavedState
	data, err := os.ReadFile(fname)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil // No state to load
		} else {
			return nil, errors.Wrapf(err, "Error reading file %s", fname)
		}
	}
	buf := bytes.NewBuffer(data)
	err = struc.Unpack(buf, &state)
	if err != nil {
		return nil, errors.Wrapf(err, "Error unpacking")
	}
	if state.Version != CniServerStateFileVersion {
		// When adding new versions, we need to keep loading old versions or some pods
		// will remain disconnected forever after an upgrade
		return nil, fmt.Errorf("unsupported save file version: %d", state.Version)
	}
	return state.Specs, nil
}
