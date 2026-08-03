# Ubuntu 24.04 software differences

The pinned upstream x64 toolset and installer chain is the software baseline;
Qiniu-specific changes are limited to container-safe service setup, the
ephemeral Actions runner, the `runner` filesystem contract, and direct Docker
daemon initialization.

The upstream Azure host kernel build string is excluded because a Qiniu
Sandbox shares its host kernel. Systemd is not PID 1, so service software is
installed and disabled but direct process startup replaces upstream
`systemctl` behavior. See `../runner-images-compatibility.json` for every
item-level decision and executable check.

Build-time service validation starts Apache in the foreground inside a detached
session and waits for the loopback HTTP port before continuing. This preserves
the upstream start/config/stop assertion without depending on systemd or
allowing a daemon or controller process to retain the Sandbox build session.

Canonical's ECR rootfs keeps its apt configuration in `sources.list`, while
the pinned upstream Ubuntu 24 setup unconditionally rewrites the Azure
image's `ubuntu.sources` path. Setup supplies an empty compatibility file at
that path and then applies the reviewed mirror list to the active apt source;
it does not skip upstream apt setup or disable TLS verification.

The upstream Podman, Buildah, and Skopeo CLI assertions remain build gates.
Only the namespace-dependent `podman networking` assertion is skipped in the
staged Pester copy because BuildKit cannot create its nested namespace; the
pinned source is not edited. Podman remains `provided`, with bridge-network
create/list/remove verified by privileged local Docker conformance and by
conformance in the Qiniu Sandbox runtime.
