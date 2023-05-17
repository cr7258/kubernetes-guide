#!/usr/bin/env bash

# Copyright 2018 The Kubernetes Authors.
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

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

cd "${KUBE_ROOT}"
# verify the dockerless build
# https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/1547-building-kubelet-without-docker/README.md
hack/verify-typecheck.sh --skip-test --tags=dockerless --ignore-dirs=test

# verify using go list
if _out="$(go list -mod=readonly -tags "dockerless" -e -json  k8s.io/kubernetes/cmd/kubelet/... \
  | grep -e dockershim)"; then
    echo "${_out}" >&2
    echo "Verify typecheck for dockerless tag failed. Found restricted packages." >&2
    exit 1
fi
