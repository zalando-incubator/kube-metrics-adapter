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
OUTPUT_BASE="$(dirname "${BASH_SOURCE[0]}")/"

# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.

OUTPUT_PKG="${GOPKG}/pkg/client"
APIS_PKG="${GOPKG}/pkg/apis"
GROUPS_WITH_VERSIONS="${CUSTOM_RESOURCE_NAME}:${CUSTOM_RESOURCE_VERSION}"

echo "Generating deepcopy funcs"
go run k8s.io/code-generator/cmd/deepcopy-gen \
  --input-dirs "${APIS_PKG}/${CUSTOM_RESOURCE_NAME}/${CUSTOM_RESOURCE_VERSION}" \
  -O zz_generated.deepcopy \
  --bounding-dirs "${APIS_PKG}" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --output-base "$OUTPUT_BASE"

echo "Generating clientset for ${GROUPS_WITH_VERSIONS} at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME:-clientset}"
go run k8s.io/code-generator/cmd/client-gen \
  --clientset-name versioned \
  --input-base "" \
  --input "${APIS_PKG}/${CUSTOM_RESOURCE_NAME}/${CUSTOM_RESOURCE_VERSION}" \
  --output-package "${OUTPUT_PKG}/clientset" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --output-base "$OUTPUT_BASE"

echo "Generating listers for ${GROUPS_WITH_VERSIONS} at ${OUTPUT_PKG}/listers"
go run k8s.io/code-generator/cmd/lister-gen \
  --input-dirs "${APIS_PKG}/${CUSTOM_RESOURCE_NAME}/${CUSTOM_RESOURCE_VERSION}" \
  --output-package "${OUTPUT_PKG}/listers" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --output-base "$OUTPUT_BASE"

echo "Generating informers for ${GROUPS_WITH_VERSIONS} at ${OUTPUT_PKG}/informers"
go run k8s.io/code-generator/cmd/informer-gen \
  --input-dirs "${APIS_PKG}/${CUSTOM_RESOURCE_NAME}/${CUSTOM_RESOURCE_VERSION}" \
  --versioned-clientset-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME:-clientset}/${CLIENTSET_NAME_VERSIONED:-versioned}" \
  --listers-package "${OUTPUT_PKG}/listers" \
  --output-package "${OUTPUT_PKG}/informers" \
  --go-header-file "${SCRIPT_ROOT}/hack/boilerplate.go.txt" \
  --output-base "$OUTPUT_BASE"

# hack to make the generated code work with Go module based projects
cp -r "$OUTPUT_BASE/$GOPKG/pkg/apis" ./pkg
cp -r "$OUTPUT_BASE/$GOPKG/pkg/client" ./pkg
rm -rf "${OUTPUT_BASE:?}${SRC}"
