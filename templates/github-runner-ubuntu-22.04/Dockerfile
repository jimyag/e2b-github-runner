# syntax=docker/dockerfile:1.7
ARG BASE_PLATFORM=linux/amd64
FROM --platform=$BASE_PLATFORM public.ecr.aws/ubuntu/ubuntu:22.04@sha256:0bc16241504efa4cf92fcc8c8039dac604c6bb832b3d8fcc24097c6b05b60b7c

ARG TEMPLATE_VERSION=20260727.1
ARG RUNNER_IMAGES_REV=e986db797519f06a2e5e53701a715cfa4c1545e8
ARG RUNNER_IMAGES_ARCHIVE_SHA256=e87ede55bbdee21ee92113124a598e64d23a21e322f65f45e3a874c563f9f4cb
ARG RUNNER_VERSION=2.334.0
ARG RUNNER_ARCHIVE_SHA256=048024cd2c848eb6f14d5646d56c13a4def2ae7ee3ad12122bee960c56f3d271
ARG SQLPACKAGE_DOTNET_TOOL_VERSION=170.4.83
ARG DOCKER_GPG_SHA256=1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570
ARG DOCKER_GPG_FINGERPRINT=9DC858229FC7DD38854AE2D88D81803C0EBFCD88

ENV DEBIAN_FRONTEND=noninteractive \
    ImageOS=ubuntu22 \
    ImageVersion=$TEMPLATE_VERSION \
    IMAGE_VERSION=$TEMPLATE_VERSION \
    IMAGE_TEMPLATE=github-runner-ubuntu-22-04 \
    RUNNER_TOOL_CACHE=/opt/hostedtoolcache \
    AGENT_TOOLSDIRECTORY=/opt/hostedtoolcache \
    HOME=/home/runner \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

COPY scripts/setup-template.sh /usr/local/share/qiniu-sandbox-runner-template/setup-template.sh
COPY scripts/ensure-docker /usr/local/share/qiniu-sandbox-runner-template/ensure-docker
COPY scripts/curl /usr/local/share/qiniu-sandbox-runner-template/curl
COPY scripts/wget /usr/local/share/qiniu-sandbox-runner-template/wget

RUN TEMPLATE_FLAVOR=versioned bash /usr/local/share/qiniu-sandbox-runner-template/setup-template.sh

USER runner
WORKDIR /home/runner/work
