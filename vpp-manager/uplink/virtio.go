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

package uplink

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/projectcalico/vpp-dataplane/v3/config"
	"github.com/projectcalico/vpp-dataplane/v3/vpp-manager/utils"
	"github.com/projectcalico/vpp-dataplane/v3/vpplink"
	"github.com/projectcalico/vpp-dataplane/v3/vpplink/types"
)

type VirtioDriver struct {
	UplinkDriverData
}

func (d *VirtioDriver) IsSupported(warn bool) (supported bool) {
	var ret bool
	supported = true
	ret = d.params.LoadedDrivers[config.DriverVfioPci]
	if !ret && warn {
		log.Warnf("did not find vfio-pci or uio_pci_generic driver")
		log.Warnf("VPP may fail to grab its interface")
	}
	supported = supported && ret

	ret = d.params.AvailableHugePages > 0
	if !ret && warn {
		log.Warnf("No hugepages configured, driver won't work")
	}
	supported = supported && ret

	ret = d.conf.Driver == config.DriverVirtioPci
	if !ret && warn {
		log.Warnf("Interface driver is <%s>, not %s", d.conf.Driver, config.DriverVirtioPci)
	}
	supported = supported && ret

	return supported
}

func (d *VirtioDriver) PreconfigureLinux() (err error) {
	newDriverName := d.spec.NewDriverName
	doSwapDriver := d.conf.DoSwapDriver
	if newDriverName == "" {
		newDriverName = config.DriverVfioPci
		doSwapDriver = config.DriverVfioPci != d.conf.Driver
	}

	if d.params.InitialVfioEnableUnsafeNoIommuMode == config.VfioUnsafeNoIommuModeNO {
		err := utils.SetVfioEnableUnsafeNoIommuMode(config.VfioUnsafeNoIommuModeYES)
		if err != nil {
			return errors.Wrapf(err, "failed to configure vfio")
		}
	}
	d.removeLinuxIfConf(true /* down */)
	if doSwapDriver {
		err = utils.SwapDriver(d.conf.PciID, newDriverName, true)
		if err != nil {
			log.Warnf("Failed to swap driver to %s: %v", newDriverName, err)
		}
	}
	return nil
}

func (d *VirtioDriver) RestoreLinux(allInterfacesPhysical bool) {
	if d.params.InitialVfioEnableUnsafeNoIommuMode == config.VfioUnsafeNoIommuModeNO {
		err := utils.SetVfioEnableUnsafeNoIommuMode(config.VfioUnsafeNoIommuModeNO)
		if err != nil {
			log.Warnf("Virtio restore error %v", err)
		}
	}
	if d.conf.PciID != "" && d.conf.Driver != "" {
		err := utils.SwapDriver(d.conf.PciID, d.conf.Driver, false)
		if err != nil {
			log.Warnf("Error swapping back driver to %s for %s: %v", d.conf.Driver, d.conf.PciID, err)
		}
	}
	if !allInterfacesPhysical {
		err := d.moveInterfaceFromNS(d.spec.InterfaceName)
		if err != nil {
			log.Warnf("Moving uplink back from NS failed %s", err)
		}
	}
	if !d.conf.IsUp {
		return
	}
	// This assumes the link has kept the same name after the rebind.
	// It should be always true on systemd based distros
	link, err := utils.SafeSetInterfaceUpByName(d.spec.InterfaceName)
	if err != nil {
		log.Warnf("Error setting %s up: %v", d.spec.InterfaceName, err)
		return
	}

	// Re-add all adresses and routes
	d.restoreLinuxIfConf(link)
}

func (d *VirtioDriver) CreateMainVppInterface(vpp *vpplink.VppLink, vppPid int, uplinkSpec *config.UplinkInterfaceSpec) (err error) {
	intf := types.VirtioInterface{
		GenericVppInterface: d.getGenericVppInterface(),
		PciID:               d.conf.PciID,
	}
	swIfIndex, err := vpp.CreateVirtio(&intf)
	if err != nil {
		return errors.Wrapf(err, "Error creating VIRTIO interface")
	}
	log.Infof("Created VIRTIO interface %d", swIfIndex)

	d.spec.SwIfIndex = swIfIndex
	err = d.TagMainInterface(vpp, swIfIndex, d.spec.InterfaceName)
	if err != nil {
		return err
	}
	return nil
}

func NewVirtioDriver(params *config.VppManagerParams, conf *config.LinuxInterfaceState, spec *config.UplinkInterfaceSpec) *VirtioDriver {
	d := &VirtioDriver{}
	d.name = NativeDriverVirtio
	d.conf = conf
	d.params = params
	d.spec = spec
	return d
}
