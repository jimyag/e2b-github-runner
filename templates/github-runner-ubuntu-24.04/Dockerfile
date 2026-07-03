ARG BASE_PLATFORM=linux/amd64
FROM --platform=$BASE_PLATFORM ubuntu:24.04

ARG DEBIAN_FRONTEND=noninteractive
ARG RUNNER_VERSION=2.334.0
ARG GO_VERSION=1.26.3
ARG NODE_MAJOR=22
ARG OPENTOFU_VERSION=1.11.5
ARG TERRAFORM_VERSION=1.14.6
ARG TASK_VERSION=3.50.0
ARG GOFUMPT_VERSION=0.8.0
ARG STATICCHECK_RELEASE=2026.1

ENV LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    RUNNER_TOOL_CACHE=/opt/hostedtoolcache \
    AGENT_TOOLSDIRECTORY=/opt/hostedtoolcache \
    GOPATH=/opt/go \
    GOBIN=/usr/local/bin \
    OPENTOFU_CACHE_DIR=/opt/hostedtoolcache/opentofu \
    TERRAFORM_CACHE_DIR=/opt/hostedtoolcache/terraform \
    PATH=/usr/local/go/bin:/usr/local/bin:/opt/go/bin:$PATH

COPY scripts/setup-template.sh /usr/local/share/qiniu-sandbox-runner-template/setup-template.sh
COPY scripts/ensure-docker /usr/local/share/qiniu-sandbox-runner-template/ensure-docker

RUN bash /usr/local/share/qiniu-sandbox-runner-template/setup-template.sh

WORKDIR /tmp
