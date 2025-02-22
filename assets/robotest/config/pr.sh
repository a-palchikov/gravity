#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

source $(dirname $0)/lib/utils.sh

# UPGRADE_MAP maps gravity version -> space separated list of linux distros to upgrade from
declare -A UPGRADE_MAP
# Use a fixed tag until we cut our first non-pre-release, as recommended_upgrade_tag skips pre-releases
# UPGRADE_MAP[$(recommended_upgrade_tag $(branch 9.0.x))]="redhat:8.4 centos:7.9 ubuntu:18 ubuntu:20"
UPGRADE_MAP[9.0.0-beta.2]="redhat:8.4 centos:7.9 ubuntu:18 ubuntu:20"
UPGRADE_MAP[8.0.0-beta.1]="redhat:7.9 centos:8.4 ubuntu:18"
UPGRADE_MAP[7.1.0-alpha.6]="ubuntu:20"

function build_upgrade_suite {
  local size='"flavor":"three","nodes":3,"role":"node"'
  local to_tarball=${INSTALLER_URL}
  local suite=''
  for release in ${!UPGRADE_MAP[@]}; do
    local from_tarball=$(tag_to_image $release)
    for os in ${UPGRADE_MAP[$release]}; do
      suite+=$(build_upgrade_step $from_tarball $to_tarball $os $size)
      suite+=' '
    done
  done
  echo -n $suite
}


function build_resize_suite {
  local suite=$(cat <<EOF
 resize={"installer_url":"${INSTALLER_URL}","nodes":1,"to":3,"flavor":"one","role":"node","state_dir":"/var/lib/telekube","os":"ubuntu:18"}
 shrink={"installer_url":"${INSTALLER_URL}","nodes":3,"flavor":"three","role":"node","os":"redhat:7.9"}
EOF
)
    echo -n $suite
}

function build_ops_suite {
  local suite=$(cat <<EOF
 install={"installer_url":"${OPSCENTER_URL}","nodes":1,"flavor":"standalone","role":"node","os":"ubuntu:18","ops_advertise_addr":"example.com:443"}
EOF
)
  echo -n $suite
}

function build_telekube_suite {
  local suite=$(cat <<EOF
 install={"installer_url":"${TELEKUBE_URL}","nodes":3,"flavor":"three","role":"node","os":"ubuntu:18"}
EOF
)
  echo -n $suite
}

function build_install_suite {
  local suite=''
  local oses="redhat:8.4 redhat:7.9 centos:8.4 centos:7.9 sles:12-sp5 sles:15-sp2 ubuntu:16 ubuntu:18 ubuntu:20 debian:9 debian:10"
  local cluster_size='"flavor":"one","nodes":1,"role":"node"'
  for os in $oses; do
    suite+=$(cat <<EOF
 install={"installer_url":"${INSTALLER_URL}",${cluster_size},"os":"${os}"}
EOF
)
  done
  suite+=' '
  echo -n $suite
}

if [[ ${1} == "upgradeversions" ]] ; then
    UPGRADE_VERSIONS=${!UPGRADE_MAP[@]}
    echo "$UPGRADE_VERSIONS"
elif [[ ${1} == "configuration" ]] ; then
    SUITE=""
    SUITE+=" $(build_telekube_suite)"
    SUITE+=" $(build_ops_suite)"
    SUITE+=" $(build_install_suite)"
    SUITE+=" $(build_resize_suite)"
    SUITE+=" $(build_upgrade_suite)"
    echo "$SUITE"
else
    echo "Unknown parameter: $1"
    exit 1
fi
