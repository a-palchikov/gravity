
FIO_VER ?= 3.15
TELEPORT_TAG := 3.2.17
ETCD_VER := v2.3.7
# Current Kubernetes version
K8S_VER ?= 1.21.0
# Kubernetes version suffix for the planet package, constructed by concatenating
# major + minor padded to 2 chars with 0 + patch also padded to 2 chars, e.g.
# 1.13.5 -> 11305, 1.13.12 -> 11312, 2.0.0 -> 20000 and so on
K8S_VER_SUFFIX := $(shell printf "%d%02d%02d" $(shell echo $(K8S_VER) | sed "s/\./ /g"))
PLANET_TAG ?= 9.0.2-$(K8S_VER_SUFFIX)
# system applications
INGRESS_APP_TAG ?= 0.0.1
STORAGE_APP_TAG ?= 0.0.4
LOGGING_APP_TAG ?= 7.1.2
MONITORING_APP_TAG ?= 7.1.4
DNS_APP_TAG ?= 7.1.2
BANDWAGON_TAG ?= 7.1.0
TILLER_VERSION ?= 2.16.12
TILLER_APP_TAG ?= 7.1.0
# abbreviated gravity version to use as a build ID
GRAVITY_VERSION := $(shell ./version.sh)
# grpc
PROTOC_VER := 3.10.0
PROTOC_PLATFORM := linux-x86_64
GOGO_PROTO_TAG := v1.3.0
GRPC_GATEWAY_TAG := v1.11.3
# URI of Wormhole container for default install
WORMHOLE_IMG ?= quay.io/gravitational/wormhole:0.3.3
# selinux
SELINUX_VERSION ?= 6.0.0
