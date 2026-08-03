# Ubuntu 22.04 software differences

The pinned upstream x64 toolset and installer chain is the software baseline;
Qiniu-specific changes are limited to container-safe service setup, the
ephemeral Actions runner, the `runner` filesystem contract, and direct Docker
daemon initialization.

The upstream Azure host kernel build string is excluded because a Qiniu
Sandbox shares its host kernel. Systemd is not PID 1, so service software is
installed and disabled but direct process startup replaces upstream
`systemctl` behavior. See `../runner-images-compatibility.json` for every
item-level decision and executable check.

Build-time service validation starts Apache in a detached session and waits
for its PID file before continuing. This preserves the upstream start/config/
stop assertion without allowing a daemon or controller process to retain the
Sandbox build session.

The upstream Podman, Buildah, and Skopeo CLI assertions remain build gates.
Only the namespace-dependent `podman networking` assertion is skipped in the
staged Pester copy because BuildKit cannot create its nested namespace; the
pinned source is not edited. Podman remains `provided`, with bridge-network
create/list/remove verified by privileged local Docker conformance and by
conformance in the Qiniu Sandbox runtime.

SqlPackage remains `provided`, but its pinned release is installed from
Microsoft's official NuGet .NET tool feed instead of the upstream evergreen
`aka.ms` archive. The archive endpoint can present an incomplete certificate
chain on container build networks; the NuGet path keeps TLS verification
enabled. The pinned upstream SqlPackage Pester assertion still runs during the
image build.
