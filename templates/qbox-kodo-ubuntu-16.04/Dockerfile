ARG BASE_PLATFORM=linux/amd64
ARG BASE_IMAGE=jimyag/qbox-kodo-ubuntu-16.04-base:runner-docker
FROM --platform=$BASE_PLATFORM $BASE_IMAGE

ARG DEBIAN_FRONTEND=noninteractive
ARG RUNNER_VERSION=2.334.0

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    RUNNER_TOOL_CACHE=/opt/hostedtoolcache \
    AGENT_TOOLSDIRECTORY=/opt/hostedtoolcache \
    GOPATH=/opt/go \
    GOBIN=/usr/local/bin \
    PATH=/usr/local/go/bin:/usr/local/bin:/opt/go/bin:$PATH

COPY scripts/setup-template.sh /usr/local/share/qiniu-sandbox-runner-template/setup-template.sh
COPY scripts/ensure-docker /usr/local/share/qiniu-sandbox-runner-template/ensure-docker

RUN bash /usr/local/share/qiniu-sandbox-runner-template/setup-template.sh

WORKDIR /tmp
