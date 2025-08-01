Calico-Vpp components (vpp-manager and agent) are configured using a common configMap. Here's an example of the configMap `calico-vpp-config` and the different configuration options it contains:


Note: keys `CALICOVPP_INTERFACE` and `CALICOVPP_NATIVE_DRIVER` are being deprecated, they are replaced by the first element of `uplinkInterfaces` field of `CALICOVPP_INTERFACES`.
Please use `CALICOVPP_INTERFACES` instead.

```yaml
---
# dedicated configmap for VPP settings
kind: ConfigMap
apiVersion: v1
metadata:
  name: calico-vpp-config
  namespace: calico-vpp-dataplane
data:

  # Configure the name of VPP's physical interface
  CALICOVPP_INTERFACE: eth1 # deprecated

  # Configures how VPP grabs the physical interface
  # available values are :
  # - ""        : will select the fastest driver among those supported for this interface
  # - avf       : use the native AVF driver
  # - virtio    : use the native virtio driver (requires hugepages)
  # - af_xdp    : use AF_XDP sock family (require at least kernel 5.4)
  # - af_packet : use AF_PACKET sock family (slow but failsafe)
  # - none      : dont configure connectivity
  CALICOVPP_NATIVE_DRIVER: "af_packet" # deprecated

  # Configures parameters for calicovpp agent and vpp manager
  CALICOVPP_INTERFACES: |-
    {
      "maxPodIfSpec": {
        "rx": 10, "tx": 10, "rxqsz": 1024, "txqsz": 1024
      },
      "defaultPodIfSpec": {
        "rx": 1, "tx":1, "isl3": true
      },
      "vppHostTapSpec": {
        "rx": 1, "tx":1, "rxqsz": 1024, "txqsz": 1024, "isl3": false
      },
      "uplinkInterfaces": [
        {
          "interfaceName": "eth1",
          "vppDriver": "af_packet",
          "mtu": 1400,
          "rxMode": "adaptive",
          "physicalNetworkName": ""
        }
      ]
    }
  CALICOVPP_INITIAL_CONFIG: |-
    {
      "vppStartupSleepSeconds": 1,
      "corePattern": "/var/lib/vpp/vppcore.%e.%p",
      "defaultGWs": "192.168.0.1",
    }

  CALICOVPP_DEBUG: |-
  {
    "servicesEnabled": true,
    "gsoEnabled": true
  }

  CALICOVPP_IPSEC: -
  {
    "crossIPSecTunnels": true,
    "nbAsyncCryptoThreads": 10,
    "extraAddresses": 0
  }

  CALICOVPP_SRV6: |-
  {
    "policyPool": "cafe::/118",
    "localsidPool": "fcff::/48",
  }
  CALICOVPP_FEATURE_GATES: |-
  {
    "memifEnabled": true,
    "vclEnabled": false,
    "multinetEnabled": true,
    "srv6Enabled": false,
    "ipsecEnabled": false
  }
```

As part of user config, you can set specific configuration for pod interfaces using pod annotations.
Here's an example:

```yaml

apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: network-blue-conf@eth1, network-red-conf@eth6
    cni.projectcalico.org/vppInterfacesSpec: |-
    {
      "eth0": {"rx": 1, "tx": 2, "isl3": true },
      "eth1": {"rx": 5, "tx": 9, "isl3": true },
      "eth6": {"rx": 3, "tx": 3, "isl3": false }
    }

```