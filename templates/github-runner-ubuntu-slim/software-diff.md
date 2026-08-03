# Ubuntu Slim software differences

Compared with the pinned upstream Ubuntu Slim report, software entries are
installed by the pinned upstream slim Dockerfile/toolset chain. Qiniu adds the
ephemeral Actions runner, `runner` filesystem contract, and a Docker daemon
because release smoke requires Docker jobs.

The upstream kernel build string is excluded: a Qiniu Sandbox shares the
Sandbox host kernel and a template cannot install or select the host kernel.
All item-level decisions and executable checks live in
`../runner-images-compatibility.json`.

Systemd is not PID 1 in the Sandbox. Services are therefore verified as
installed/disabled, while `ensure-docker` initializes Docker directly and
maintains non-root socket access.

Canonical's ECR rootfs keeps its apt configuration in `sources.list`, while
the pinned upstream Slim setup unconditionally rewrites the Azure image's
`ubuntu.sources` path. Setup supplies an empty compatibility file at that path
and then applies the reviewed mirror list to the active apt source; it does not
skip the upstream apt setup or disable TLS verification.
