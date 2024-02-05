# This is a wrapper to set common variables
#
# All make targets related to common variables are defined in this file.

# ====================================================================================================
# Configure Make itself:
# ====================================================================================================

# Turn off .INTERMEDIATE file removal by marking all files as
# .SECONDARY.  .INTERMEDIATE file removal is a space-saving hack from
# a time when drives were small; on modern computers with plenty of
# storage, it causes nothing but headaches.
#
# https://news.ycombinator.com/item?id=16486331
.SECONDARY:

# SHELL:=/bin/bash
SHELL:=/bin/bash -o pipefail
# SHELL:=/bin/bash -o pipefail -x

# ====================================================================================================
# ROOT Options:
# ====================================================================================================

# Set Root Directory Path
ifeq ($(origin ROOT_DIR),undefined)
ROOT_DIR := $(abspath $(shell pwd -P))
endif

# ====================================================================================================
# ENV Options:
# ====================================================================================================

OCI_REGISTRY ?= oci://docker.io/hejianmin
# REGISTRY is the image registry to use for build and push image targets.
REGISTRY ?= docker.io/hejianmin
# IMAGE_NAME is the name of image
# Use virtnet-dev in default when developing
# Use virtnet when releasing an image.
IMAGE_NAME_AGENT ?= virtnet-agent
IMAGE_NAME_CTRL ?= virtnet-ctrl
IMAGE_NAME_PLUGINS ?= virtnet-plugins
# HELM_NAME is the name of helm chart
HELM_NAME ?= chart-virtnet
# IMAGE is the image URL for build and push image targets.
IMAGE_AGENT ?= $(REGISTRY)/$(IMAGE_NAME_AGENT)
IMAGE_CTRL ?= $(REGISTRY)/$(IMAGE_NAME_CTRL)
IMAGE_PLUGINS ?= $(REGISTRY)/$(IMAGE_NAME_PLUGINS)
# Version is the tag to use for build and push image targets.
VERSION ?= $(shell git describe --tags --abbrev=8)

.PHONY: help
help: ## Show this help info.
	@$(LOG_TARGET)
	@echo -e "VirtNet is a kubevirt virtual machine network plugin\n"
	@echo -e "Usage:\n  make \033[36m<Target>\033[0m \033[36m<Option>\033[0m\n\nTargets:"
	@awk 'BEGIN {FS = ":.*##"; printf ""} /^[a-zA-Z_0-9\.-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

# ====================================================================================================
# Includes:
# ====================================================================================================
include tools/make/image.mk
include tools/make/helm.mk
include tools/make/kube.mk
include tools/make/grpc.mk

# Log the running target
LOG_TARGET = echo -e "\033[0;32m===========> Running $@ ... \033[0m"

# Log debugging info
define log
echo -e "\033[36m===========>$1\033[0m"
endef

define errorlog
echo -e "\033[0;31m===========>$1\033[0m"
endef
