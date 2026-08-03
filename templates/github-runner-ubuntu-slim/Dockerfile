# syntax=docker/dockerfile:1.7
ARG BASE_PLATFORM=linux/amd64
FROM --platform=$BASE_PLATFORM public.ecr.aws/ubuntu/ubuntu:24.04@sha256:be20a0347f238b7d373edddc55923443b21dd9a60277bf8a93e43458cd0bf2fc

ARG TEMPLATE_VERSION=20260727.1
ARG RUNNER_IMAGES_REV=e986db797519f06a2e5e53701a715cfa4c1545e8
ARG RUNNER_IMAGES_ARCHIVE_SHA256=e87ede55bbdee21ee92113124a598e64d23a21e322f65f45e3a874c563f9f4cb
ARG RUNNER_VERSION=2.334.0
ARG RUNNER_ARCHIVE_SHA256=048024cd2c848eb6f14d5646d56c13a4def2ae7ee3ad12122bee960c56f3d271
ARG AZCOPY_VERSION=10.32.6
ARG AZCOPY_DEB_SHA256=1a5078a8260ba7524a4400c602519d36905e592cd87a1db91a30af0da528fc86
ARG DOCKER_GPG_SHA256=1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570
ARG DOCKER_GPG_FINGERPRINT=9DC858229FC7DD38854AE2D88D81803C0EBFCD88

ENV DEBIAN_FRONTEND=noninteractive \
    ImageOS=ubuntu24 \
    ImageVersion=$TEMPLATE_VERSION \
    IMAGE_VERSION=$TEMPLATE_VERSION \
    IMAGE_TEMPLATE=github-runner-ubuntu-slim \
    RUNNER_TOOL_CACHE=/opt/hostedtoolcache \
    AGENT_TOOLSDIRECTORY=/opt/hostedtoolcache \
    HOME=/home/runner \
    NVM_DIR=/home/runner/.nvm \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

COPY scripts/setup-template.sh /usr/local/share/qiniu-sandbox-runner-template/setup-template.sh
COPY scripts/ensure-docker /usr/local/share/qiniu-sandbox-runner-template/ensure-docker
COPY scripts/curl /usr/local/share/qiniu-sandbox-runner-template/curl

RUN TEMPLATE_FLAVOR=slim bash /usr/local/share/qiniu-sandbox-runner-template/setup-template.sh

USER runner
WORKDIR /home/runner/work
