# Ubuntu 24.04 software differences

The pinned upstream x64 report is the comparison inventory. The guaranteed
software baseline is the pinned Ubuntu Slim-compatible core on Ubuntu 24.04,
plus Apache, Podman, Buildah, Skopeo, Ninja, pipx packages, the ephemeral
Actions runner, the `runner` filesystem contract, and direct Docker daemon
initialization. The current Qiniu Sandbox public-template build allocation has
a 22,222-MiB root disk; the complete GitHub-hosted runner toolset exceeds that
limit. Full-image-only items are therefore `excluded` from the guarantee and
can be installed at job time or included in a custom template.

The upstream Azure host kernel build string is excluded because a Qiniu
Sandbox shares its host kernel. Systemd is not PID 1, so service software is
not guaranteed merely because it appears in the full upstream report. Apache
uses direct process startup instead of upstream `systemctl` behavior. See
`../runner-images-compatibility.json` for every item-level decision and
executable check.

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
