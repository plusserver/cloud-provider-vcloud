#!/usr/bin/env bash

# Copyright 2020 The Kubernetes Authors.
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


CLOUD_PROVIDER_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

echo "$CLOUD_PROVIDER_ROOT"

GOPATH=$HOME/go

export KUBE_ROOT="$GOPATH/src/k8s.io/kubernetes"
export CLOUD_PROVIDER=vCloud
export EXTERNAL_CLOUD_PROVIDER=true
export CLOUD_CONFIG=$(pwd)/cloudconfig.yml
export EXTERNAL_CLOUD_PROVIDER_BINARY="${CLOUD_PROVIDER_ROOT}/vcloud-cloud-controller-manager"

# Stop right away if the build fails
set -e

make -C "${CLOUD_PROVIDER_ROOT}"

write_cloudconfig() {
    rm -f "$CLOUD_CONFIG"
    cat <<EOF >> "$CLOUD_CONFIG"
user: ""
password: ""
org: ""
href: ""
vdc: ""
insecure: false
gateway: ""
EOF
}

write_cloudconfig

"$KUBE_ROOT"/hack/local-up-cluster.sh "$@"
