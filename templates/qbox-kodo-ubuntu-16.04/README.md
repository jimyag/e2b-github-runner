# qbox kodo Ubuntu 16.04 Runner Template

Legacy Qiniu sandbox runner template for qbox/kodo-style GitHub Actions jobs that still need Ubuntu 16.04 era system dependencies.

## Base Image

`Dockerfile` starts from:

```text
jimyag/qbox-kodo-ubuntu-16.04-base:runner-docker
```

The base image is defined by `base.Dockerfile` in this directory. It installs the Ubuntu 16.04 apt dependencies, Docker, the default `user` account, sudo access, and shared runner directories. The Qiniu sandbox template layer then runs `scripts/setup-template.sh` to install the GitHub Actions runner, pinned Go toolchains, and final validation.

Build and push the base image before rebuilding the Qiniu sandbox template when `base.Dockerfile` changes:

```bash
task qbox-kodo-base-build
task qbox-kodo-base-push
```

## Qiniu Sandbox Template

Build the Qiniu sandbox template with:

```bash
task qbox-kodo-template-build-prod
```

The Taskfile copies `qshell.sandbox.toml` to a temporary file before calling `qshell sandbox template build`, so any generated `template_id` is not written back into the tracked config. The production template name is `qbox-kodo-ubuntu-16-04`.
