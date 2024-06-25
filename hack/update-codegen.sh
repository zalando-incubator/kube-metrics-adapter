#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

SRC="github.com"
GOPKG="${SRC}/zalando-incubator/kube-metrics-adapter"
CUSTOM_RESOURCE_NAME="zalando.org"
CUSTOM_RESOURCE_VERSION="v1"

SCRIPT_ROOT="$(dirname "${BASH_SOURCE[0]}")/.."

OUTPUT_DIR="pkg/client"
OUTPUT_PKG="${GOPKG}/${OUTPUT_DIR}"
APIS_PKG="${GOPKG}/pkg/apis"
GROUPS_WITH_VERSIONS="${CUSTOM_RESOURCE_NAME}:${CUSTOM_RESOURCE_VERSION}"

echo "Generating deepcopy funcs"
go run k8s.io/code-generator/cmd/deepcopy-gen \
  --output-file zz_generated.deepcopy.go \
  --bounding-dirs "${APIS_PKG}" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  "${APIS_PKG}/${CUSTOM_RESOURCE_NAME}/${CUSTOM_RESOURCE_VERSION}"

echo "Generating clientset for ${GROUPS_WITH_VERSIONS} at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME:-clientset}"
go run k8s.io/code-generator/cmd/client-gen \
  --clientset-name versioned \
  --input-base "" \
  --input "${APIS_PKG}/${CUSTOM_RESOURCE_NAME}/${CUSTOM_RESOURCE_VERSION}" \
  --output-pkg "${OUTPUT_PKG}/clientset" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --output-dir "${OUTPUT_DIR}/clientset"

echo "Generating listers for ${GROUPS_WITH_VERSIONS} at ${OUTPUT_PKG}/listers"
go run k8s.io/code-generator/cmd/lister-gen \
  --output-pkg "${OUTPUT_PKG}/listers" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --output-dir "${OUTPUT_DIR}/listers" \
  "${APIS_PKG}/${CUSTOM_RESOURCE_NAME}/${CUSTOM_RESOURCE_VERSION}"

echo "Generating informers for ${GROUPS_WITH_VERSIONS} at ${OUTPUT_PKG}/informers"
go run k8s.io/code-generator/cmd/informer-gen \
  --versioned-clientset-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME:-clientset}/${CLIENTSET_NAME_VERSIONED:-versioned}" \
  --listers-package "${OUTPUT_PKG}/listers" \
  --output-pkg "${OUTPUT_PKG}/informers" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --output-dir "${OUTPUT_DIR}/informers" \
  "${APIS_PKG}/${CUSTOM_RESOURCE_NAME}/${CUSTOM_RESOURCE_VERSION}"
