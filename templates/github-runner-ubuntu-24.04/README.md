# GitHub Runner Ubuntu 24.04 Qiniu Sandbox Template

This template is a lightweight GitHub Actions self-hosted runner image for Qiniu sandboxes.

It intentionally does not copy the full GitHub-hosted Ubuntu 24.04 image. The official image includes many heavy stacks, browsers, SDKs, Android tooling, databases, and cloud CLIs. For this runner service, the template focuses on the pieces needed for stable Go CI jobs:

- `linux/amd64` Ubuntu 24.04 base, even when building from a Mac arm64 machine.
- Core shell, archive, network, build, Git, and Git LFS tools.
- `gawk`, because the GitHub runner update script uses `awk`.
- Python 3 and Node.js/npm for common Actions runtime needs.
- LAS CI tooling preinstalled in the template: Go 1.26.3, Node.js 22, GitHub CLI, Docker CLI/daemon packages, rclone, Go Task, gofumpt, goimports, staticcheck 0.7.0, OpenTofu 1.11.5, and Terraform 1.14.6.
- A writable `user` account and writable temp-backed runner homes.
- Preinstalled GitHub Actions runner package and runtime dependencies under `/opt/actions-runner`.
- Writable tool cache directory under `/opt/hostedtoolcache`.

Reference:

- GitHub hosted runner software list: https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md
- Qiniu sandbox qshell reference: https://developer.qiniu.com/las/13442/sandbox-qshell

## Build with qshell

```bash
task template-build-prod
```

For an isolated development tag:

```bash
task template-build-dev
```

The Taskfile copies `qshell.sandbox.toml` to a temporary file before calling `qshell sandbox template build`, so any generated `template_id` is not written back into the tracked config. Both production and development template builds request `8` vCPUs and `8192` MiB of memory. New sandboxes inherit those resources from the rebuilt template.

The build prints a template ID and template name. Use the production template name or ID as:

```bash
export SANDBOX_TEMPLATE_ID="<template-id>"
```

Use a configured qshell account or set `QINIU_API_KEY` for template builds. Do not use the old E2B SDK build scripts for this template.

The Dockerfile defaults the base image platform with:

```dockerfile
ARG BASE_PLATFORM=linux/amd64
FROM --platform=$BASE_PLATFORM ubuntu:24.04
```

and installs the `actions-runner-linux-x64` package plus its Linux runtime dependencies. This is intentional because the sandbox runtime expects Linux amd64; do not switch it to arm64 for local Mac builds.

## Smoke Test

```bash
qshell sandbox create <template-id-or-name> --detach
qshell sandbox list
```

Then run a workflow using:

```yaml
runs-on: [self-hosted, e2b]
```

## Notes

The runner service requires `/opt/actions-runner/config.sh` and `/opt/actions-runner/run.sh` to exist and copies `/opt/actions-runner` into `/tmp/actions-runner`. It does not download the runner tarball at runtime.

The runner service also sets:

```bash
HOME=/tmp/runner-home
XDG_CONFIG_HOME=/tmp/runner-home/.config
```

This keeps Git config access away from the earlier broken `/home/user/.config` path. Go-specific paths are left to `actions/setup-go` defaults.

For LAS workflows, Docker is installed in the template and the runner startup script calls `/usr/local/bin/ensure-docker` before registering the Actions runner. The helper starts `dockerd` when possible and logs `/tmp/dockerd.log` if the sandbox runtime does not allow a Docker daemon to become ready.
