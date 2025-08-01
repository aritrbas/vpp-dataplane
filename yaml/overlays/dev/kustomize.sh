#!/bin/bash

# Copyright (c) 2020 Cisco and/or its affiliates.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at:
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

SCRIPTDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

function is_ip6 () {
  if [[ $1 =~ .*:.*/.* ]]; then
	echo "true"
  else
	echo "false"
  fi
}

function green ()
{
  printf "\e[0;32m$1\e[0m\n"
}

function red ()
{
  printf "\e[0;31m$1\e[0m\n"
}

function get_cluster_service_cidr ()
{
  kubectl cluster-info dump | grep -m 1 service-cluster-ip-range | cut -d '=' -f 2 | cut -d '"' -f 1
}

function get_available_node_names ()
{
  kubectl get nodes -o go-template --template='{{range .items}}{{printf "%s\n" .metadata.name}}{{end}}'
}

function get_node_addresses ()
{
  kubectl get nodes $1 -o go-template --template='{{printf "%s\n" .spec.podCIDR}}'
}

function kustomize_parse_variables ()
{
  # This sets the following vars unless provided
  # CLUSTER_POD_CIDR4
  # CLUSTER_POD_CIDR6
  # SERVICE_CIDR
  # IP_VERSION

  if [ x${CLUSTER_POD_CIDR4}${CLUSTER_POD_CIDR6} = x ]; then
	FIRST_NODE=$(get_available_node_names | head -1)
	for ip in $(get_node_addresses $FIRST_NODE) ; do
	  if [[ $(is_ip6 $ip) == true ]]; then
		  CLUSTER_POD_CIDR6=$ip
	  else
		  CLUSTER_POD_CIDR4=$ip
	  fi
	done
  fi

  if [ x${IP_VERSION} = x ]; then
	IP_VERSION=""
	if [[ x$CLUSTER_POD_CIDR4 != x ]]; then
  	 IP_VERSION=4
	fi
	if [[ x$CLUSTER_POD_CIDR6 != x ]]; then
  	 IP_VERSION=${IP_VERSION}6
	fi
  fi

  if [ x${SERVICE_CIDR} = x ]; then
	SERVICE_CIDR=$(get_cluster_service_cidr)
  fi
}

function get_vpp_conf ()
{
	echo "unix {
		nodaemon
		full-coredump
		log /var/run/vpp/vpp.log
		cli-listen /var/run/vpp/cli.sock
		pidfile /run/vpp/vpp.pid
	}
	api-trace { on }
	cpu { main-core ${MAINCORE} workers ${WRK} }
	socksvr {
		socket-name /var/run/vpp/vpp-api.sock
	}
	buffers {
	  buffers-per-numa 65536
	}
	plugins {
		plugin default { enable }
		plugin calico_plugin.so { enable }
		plugin dpdk_plugin.so { disable }
		plugin ping_plugin.so { disable }
	}"
}

function get_initial_config ()
{
    echo "{
      \"vppStartupSleepSeconds\": ${CALICOVPP_VPP_STARTUP_SLEEP:-0},
      \"corePattern\": \"${CALICOVPP_CORE_PATTERN:-/var/lib/vpp/vppcore.%e.%p}\",
      \"defaultGWs\": \"${CALICOVPP_DEFAULT_GW}\",
      \"redirectToHostRules\": [
      {
        \"proto\": \"${CALICOVPP_REDIRECT_PROTO:-udp}\",
        \"port\": ${CALICOVPP_REDIRECT_PORT:-53},
        \"ip\": \"${CALICOVPP_REDIRECT_IP:-172.18.0.1}\"
      }
    ]
    }"
}

function get_feature_gates ()
{
    echo "{
      \"memifEnabled\": ${CALICOVPP_ENABLE_MEMIF:-"false"},
      \"vclEnabled\": ${CALICOVPP_ENABLE_VCL:-"false"},
      \"multinetEnabled\": ${CALICOVPP_ENABLE_MULTINET:-"false"},
      \"ipsecEnabled\": ${CALICOVPP_IPSEC_ENABLED:-"false"}
    }"
}

function get_debug ()
{
    echo "{
      \"servicesEnabled\": ${CALICOVPP_DEBUG_ENABLE_SERVICES:-true},
      \"gsoEnabled\": ${CALICOVPP_DEBUG_ENABLE_GSO:-true}
    }"
}

function get_interfaces ()
{
    echo "{
    \"defaultPodIfSpec\": {
      \"rx\": ${CALICOVPP_RX_QUEUES:-1},
      \"tx\": ${CALICOVPP_TX_QUEUES:-1},
      \"isl3\": true,
      \"rxMode\": \"${CALICOVPP_RX_MODE:-adaptive}\"
    },
    \"vppHostTapSpec\": {
      \"rx\": ${CALICOVPP_TAP_RX_QUEUES:-1},
      \"tx\": ${CALICOVPP_TAP_TX_QUEUES:-1},
      \"rxqsz\": 1024,
      \"txqsz\": 1024,
      \"isl3\": false,
      \"rxMode\": \"${CALICOVPP_TAP_RX_MODE:-adaptive}\"
    },
    \"uplinkInterfaces\": [
      {
        \"interfaceName\": \"${CALICOVPP_MAIN_INTERFACE:-eth0}\",
        \"vppDriver\": \"${CALICOVPP_MAIN_NATIVE_DRIVER:-af_packet}\",
        \"rxMode\": \"${CALICOVPP_RX_MODE:-adaptive}\"
      }
    ]}"
}

function get_installation_cidrs ()
{
	if [[ $IP_VERSION == 4 ]]; then
	  echo "
    - cidr: ${CLUSTER_POD_CIDR4}
      encapsulation: ${CALICO_ENCAPSULATION_V4:-IPIP}
      natOutgoing: ${CALICO_NAT_OUTGOING:-Enabled}"
	elif [[ $IP_VERSION == 6 ]]; then
	  echo "
    - cidr: ${CLUSTER_POD_CIDR6}
      encapsulation: ${CALICO_ENCAPSULATION_V6:-None}
      natOutgoing: ${CALICO_NAT_OUTGOING:-Enabled}"
	else
	  echo "
    - cidr: ${CLUSTER_POD_CIDR4}
      encapsulation: ${CALICO_ENCAPSULATION_V4:-IPIP}
      natOutgoing: ${CALICO_NAT_OUTGOING:-Enabled}
    - cidr: ${CLUSTER_POD_CIDR6}
      encapsulation: ${CALICO_ENCAPSULATION_V6:-None}
      natOutgoing: ${CALICO_NAT_OUTGOING:-Enabled}"
	fi
}

function is_v4_v46_v6 ()
{
	if [[ x$IP_VERSION == x4 ]]; then
		echo $1
	elif [[ x$IP_VERSION == x46 ]]; then
		echo $2
	else
		echo $3
  	fi
}

function get_empty_object ()
{
	echo "{}"
}

calico_create_template ()
{
  kustomize_parse_variables
  >&2 green "Installing CNI for"
  >&2 green "pod cidr     : ${CLUSTER_POD_CIDR4},${CLUSTER_POD_CIDR6}"
  >&2 green "service cidr : $SERVICE_CIDR"
  >&2 green "is ip6       : $(is_v4_v46_v6 v4 v46 v6)"
  if [ x${CLUSTER_POD_CIDR4}${CLUSTER_POD_CIDR6} = x ]; then
  	>&2 red "No CLUSTER_POD_CIDR[46] set, exiting"
  	exit 1
  fi
  if [ x${IP_VERSION} = x ]; then
  	>&2 red "No IP_VERSION set, exiting"
  	exit 1
  fi
  if [[ x$SERVICE_CIDR = x ]]; then
  	>&2 red "No SERVICE_CIDR set, exiting"
  	exit 1
  fi

  WRK=${WRK:-0}
  MAINCORE=${MAINCORE:-12}
  DPDK=${DPDK:-true}

  ## Templating calico-vpp-dev-patch.yaml ##
  export CALICO_AGENT_IMAGE=${CALICO_AGENT_IMAGE:-docker.io/calicovpp/agent:latest}
  export CALICO_VPP_IMAGE=${CALICO_VPP_IMAGE:-docker.io/calicovpp/vpp:latest}
  export CALICO_VERSION_TAG=${CALICO_VERSION_TAG:-v3.20.0}
  export IMAGE_PULL_POLICY=${IMAGE_PULL_POLICY:-IfNotPresent}
  export REPO_DIRECTORY=$(readlink -f ${SCRIPTDIR}/../../..)

  ## Templating multinet-monitor-dev-patch.yaml ##
  export MULTINET_MONITOR_IMAGE=${MULTINET_MONITOR_IMAGE:-docker.io/calicovpp/multinet-monitor:latest}

  ## Templating installation-dev.yaml ##
  export CALICO_MTU=${CALICO_MTU:-0}
  export CALICOVPP_MAIN_INTERFACE=${CALICOVPP_MAIN_INTERFACE:-eth0}
  export INSTALLATION_CIDRS=$(get_installation_cidrs)

  ## Templating calico-vpp-dev-configmap.yaml ##
  export SERVICE_PREFIX=$SERVICE_CIDR
  export CALICOVPP_SWAP_DRIVER='"'${CALICOVPP_SWAP_DRIVER}'"' # DEPRECATED
  export CALICOVPP_CONFIG_EXEC_TEMPLATE='"'${CALICOVPP_CONFIG_EXEC_TEMPLATE}'"'
  export CALICOVPP_INIT_SCRIPT_TEMPLATE='"'${CALICOVPP_INIT_SCRIPT_TEMPLATE}'"'
  export CALICOVPP_IPSEC_IKEV2_PSK='"'${CALICOVPP_IPSEC_IKEV2_PSK:-keykeykey}'"'
  export CALICOVPP_LOG_LEVEL='"'${CALICOVPP_LOG_LEVEL}'"'
  export CALICOVPP_BGP_LOG_LEVEL='"'${CALICOVPP_BGP_LOG_LEVEL}'"'
  export CALICOVPP_CONFIG_TEMPLATE="$(indent_variable "${CALICOVPP_CONFIG_TEMPLATE:-$(get_vpp_conf)}")"
  export CALICOVPP_INITIAL_CONFIG="$(indent_variable "${CALICOVPP_INITIAL_CONFIG:-$(get_initial_config)}")"
  export CALICOVPP_INTERFACES="$(indent_variable "${CALICOVPP_INTERFACES:-$(get_interfaces)}")"
  export CALICOVPP_DEBUG="$(indent_variable "${CALICOVPP_DEBUG:-$(get_debug)}")"
  export CALICOVPP_FEATURE_GATES="$(indent_variable "${CALICOVPP_FEATURE_GATES:-$(get_feature_gates)}")"
  export CALICOVPP_IPSEC="$(indent_variable "${CALICOVPP_IPSEC:-"{}"}")"
  export CALICOVPP_SRV6="$(indent_variable "${CALICOVPP_SRV6:-"{}"}")"
  export DEBUG='"'${DEBUG}'"' # should we run VPP release or debug ?

  cd $SCRIPTDIR

cat > kustomization.yaml <<EOF
bases:
  - ../../base
  - installation-dev.yaml
components:
${CALICOVPP_ENABLE_MULTINET:+  - ../../components/multinet}
patchesStrategicMerge:
  - calico-vpp-dev-configmap.yaml
  - calico-vpp-dev-patch.yaml
${CALICOVPP_ENABLE_MULTINET:+  - multinet-monitor-dev-patch.yaml}
${CALICOVPP_DISABLE_HUGEPAGES:+  - calico-vpp-nohuge.yaml}
EOF

  kubectl kustomize . | envsubst | \
	tee /tmp/calico-vpp.yaml > /dev/null

  rm kustomization.yaml
}

function indent_variable () {
  echo "|-"
  printf "%s\n" "$1" | (while read; do echo "    $REPLY"; done)
}

function calico_up_cni ()
{
  calico_create_template $@
  if [ x$DISABLE_KUBE_PROXY = xyes ]; then
    kubectl patch ds -n kube-system kube-proxy -p '{"spec":{"template":{"spec":{"nodeSelector":{"non-calico": "true"}}}}}'
  fi
  if [ -t 1 ]; then
	kubectl apply -f /tmp/calico-vpp.yaml
  else
  	cat /tmp/calico-vpp.yaml
  fi
}

function calico_down_cni ()
{
  calico_create_template
  if [ x$DISABLE_KUBE_PROXY = xy ]; then
    kubectl patch ds -n kube-system kube-proxy --type merge -p '{"spec":{"template":{"spec":{"nodeSelector":{"non-calico": null}}}}}'
  fi
  kubectl delete -f /tmp/calico-vpp.yaml
}

function print_usage_and_exit ()
{
    echo "Usage:"
    echo "kustomize.sh up            - Install calico dev cni"
    echo "kustomize.sh dn            - Delete calico dev cni"
    echo
    exit 0
}

kustomize_cli ()
{
  if [[ "$1" = "up" ]]; then
	shift
	calico_up_cni $@
  elif [[ "$1" = "dn" ]]; then
	shift
	calico_down_cni $@
  else
  	print_usage_and_exit
  fi
}

kustomize_cli $@
