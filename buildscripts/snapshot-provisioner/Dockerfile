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

FROM openebs/linux-utils:2.12.x-ci

RUN apk add --no-cache \
    bash \
    net-tools \
    mii-tool \
    procps \
    libc6-compat \
    ca-certificates


COPY snapshot-provisioner /
# Root CA certificates
COPY etc /etc
COPY usr /usr

ARG ARCH
ARG DBUILD_DATE
ARG DBUILD_REPO_URL
ARG DBUILD_SITE_URL

LABEL org.label-schema.schema-version="1.0"
LABEL org.label-schema.name="snapshot-provisioner"
LABEL org.label-schema.description="OpenEBS Dynamic cStor Snapshot Provisioner"
LABEL org.label-schema.build-date=$DBUILD_DATE
LABEL org.label-schema.vcs-url=$DBUILD_REPO_URL
LABEL org.label-schema.url=$DBUILD_SITE_URL

ENTRYPOINT ["/snapshot-provisioner"]
