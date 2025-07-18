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

type Vmxnet3Driver struct {
	UplinkDriverData
}

func (d *Vmxnet3Driver) IsSupported(warn bool) (supported bool) {
	var ret bool
	supported = true

	ret = d.conf.Driver == config.DriverVmxNet3
	if !ret && warn {
		log.Warnf("Interface driver is <%s>, not %s", d.conf.Driver, config.DriverVmxNet3)
	}
	supported = supported && ret

	return supported
}

func (d *Vmxnet3Driver) PreconfigureLinux() (err error) {
	if d.params.InitialVfioEnableUnsafeNoIommuMode == config.VfioUnsafeNoIommuModeNO {
		err := utils.SetVfioEnableUnsafeNoIommuMode(config.VfioUnsafeNoIommuModeYES)
		if err != nil {
			return errors.Wrapf(err, "Vmxnet3 preconfigure error")
		}
	}
	d.removeLinuxIfConf(true /* down */)
	driverName, err := utils.GetDriverNameFromPci(d.conf.PciID)
	if err != nil {
		return errors.Wrapf(err, "Couldnt get VF driver Name for %s", d.conf.PciID)
	}
	if driverName != config.DriverVfioPci {
		err := utils.SwapDriver(d.conf.PciID, config.DriverVfioPci, true)
		if err != nil {
			return errors.Wrapf(err, "Couldnt swap %s to vfio_pci", d.conf.PciID)
		}
	}
	return nil
}

func (d *Vmxnet3Driver) RestoreLinux(allInterfacesPhysical bool) {
	if d.conf.PciID != "" && d.conf.Driver != "" {
		err := utils.SwapDriver(d.conf.PciID, d.conf.Driver, true)
		if err != nil {
			log.Warnf("Error swapping back driver to %s for %s: %v", d.conf.Driver, d.conf.PciID, err)
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

	if d.params.InitialVfioEnableUnsafeNoIommuMode == config.VfioUnsafeNoIommuModeNO {
		err = utils.SetVfioEnableUnsafeNoIommuMode(config.VfioUnsafeNoIommuModeNO)
		if err != nil {
			log.Errorf("Vmxnet3 restore error %s", err)
		}
	}
}

func (d *Vmxnet3Driver) CreateMainVppInterface(vpp *vpplink.VppLink, vppPid int, uplinkSpec *config.UplinkInterfaceSpec) (err error) {
	intf := types.Vmxnet3Interface{
		GenericVppInterface: d.getGenericVppInterface(),
		EnableGso:           *config.GetCalicoVppDebug().GSOEnabled,
		PciID:               d.conf.PciID,
	}
	swIfIndex, err := vpp.CreateVmxnet3(&intf)
	if err != nil {
		return errors.Wrapf(err, "Error creating Vmxnet3 interface")
	}

	log.Infof("Created Vmxnet3 interface %d", swIfIndex)

	d.spec.SwIfIndex = swIfIndex
	err = d.TagMainInterface(vpp, swIfIndex, d.spec.InterfaceName)
	if err != nil {
		return err
	}
	return nil
}

func NewVmxnet3Driver(params *config.VppManagerParams, conf *config.LinuxInterfaceState, spec *config.UplinkInterfaceSpec) *Vmxnet3Driver {
	d := &Vmxnet3Driver{}
	d.name = NativeDriverVmxnet3
	d.conf = conf
	d.params = params
	d.spec = spec
	return d
}
