# Managed GitHub runner templates

## Supported templates

| Workflow label | Physical template | Upstream baseline | Support channel | Initial publication state |
| --- | --- | --- | --- | --- |
| `ubuntu-slim` | `github-runner-ubuntu-slim` | Ubuntu Slim x64 | stable | development |
| `ubuntu-22.04` | `github-runner-ubuntu-22-04` | Ubuntu 22.04 x64 | follows upstream deprecation | development |
| `ubuntu-24.04` | `github-runner-ubuntu-24-04` | Ubuntu 24.04 x64 | stable | development |
| `ubuntu-26.04` | `github-runner-ubuntu-26-04` | Ubuntu 26.04 x64 | preview | development |
| `ubuntu-latest` | `github-runner-ubuntu-24-04` | Ubuntu 24.04 x64 | stable logical mapping | development |

The image-specific reports are [Ubuntu Slim](github-runner-ubuntu-slim/software-diff.md),
[Ubuntu 22.04](github-runner-ubuntu-22.04/software-diff.md),
[Ubuntu 24.04](github-runner-ubuntu-24.04/software-diff.md), and
[Ubuntu 26.04](github-runner-ubuntu-26.04/software-diff.md).
`ubuntu-latest` is a logical mapping to the 24.04 physical template and has no
fifth physical template directory.

Publication state is restricted to `development`, `published`, or `verified`.
`published` means the physical template is public in both supported regions.
`verified` additionally requires Task 10 to attach successful two-region smoke
evidence to Issue #38. A local build or one-region publication remains
`development`.

## Compatibility contract

Compatibility is independently verified across these dimensions:

- Linux x86_64 OS/release identity;
- the non-root `runner` user, `/home/runner`, `/home/runner/work`, and the
  preinstalled runner under `/opt/actions-runner`;
- image identification variables, `RUNNER_TOOL_CACHE`,
  `AGENT_TOOLSDIRECTORY`, and upstream environment variables;
- the `/opt/hostedtoolcache` layout and every item in the pinned upstream
  software inventory, either as a guaranteed item or an explained exclusion;
- Docker daemon/socket behavior, Apache direct-process behavior, outbound
  HTTPS, runner registration, job execution, and cleanup.

The contract has no compatibility percentage. Every upstream item is either
`provided` with an executable verification command or `excluded` with a
specific Qiniu Sandbox limitation.

The current public-template build allocation exposes a 22,222-MiB root disk.
The complete GitHub-hosted runner image exceeds that allocation. The three
versioned templates therefore guarantee the pinned Ubuntu Slim-compatible
core on the requested Ubuntu release, plus Apache, Podman, Buildah, Skopeo,
Ninja, pinned Pester for build-time validation, and the Qiniu runner contract.
Full-image-only tools remain in the manifest as `excluded`; they are not
guaranteed to be preinstalled and should be installed in the workflow or
included in a custom template. `excluded` describes the support contract, not
a promise that the executable is absent.

All four templates install checksum-pinned AzCopy 10.32.6 from Microsoft's
official Ubuntu package pool instead of the upstream floating `aka.ms`
archive. They intentionally do not prewarm the GitHub action archive cache;
the runner resolves actions through the normal job-time GitHub protocol, so
this changes build size and network exposure rather than workflow semantics.
After Azure CLI is installed, they also install the upstream-selected Azure
DevOps extension 1.0.6 from its official Microsoft Blob wheel with a pinned
checksum and bounded curl retries; versioned templates retain the upstream
Pester assertion for that extension.

All four templates install Bicep 0.45.15 from Microsoft's signed NuGet
package instead of resolving and downloading a floating GitHub release asset.
The package checksum is pinned in each Dockerfile. The three versioned
templates run the upstream Bicep Pester assertion after installation; the Slim
template, whose pinned upstream tree has no Pester invocation helper, verifies
the installed CLI version directly.

Other GitHub release assets selected by the pinned upstream installers are
resolved through GitHub's release API and downloaded through the corresponding
official asset API URL. This avoids a separate browser-download handshake while
retaining the upstream version selection and checksum validation.

Git LFS comes from the configured Ubuntu archive instead of adding the
packagecloud repository. This keeps the package on the OS-supported channel,
avoids another build-time repository key, and retains the upstream Git LFS
Pester assertion on the three versioned templates. The Slim template verifies
the installed Git LFS CLI directly.

All four templates install NVM 0.40.6 from the checksum-pinned official tag
archive instead of cloning the repository during the build. The resulting
profile setup and system-Node default match the pinned upstream installer.

Google Cloud CLI installation first follows the pinned upstream
`actions/runner-images` APT path. If that repository remains unavailable after
bounded retries, the build falls back to Google's official versioned x86_64
archive. The fallback version and SHA-256 digest are pinned in each Dockerfile,
and the resulting `gcloud` command is verified before the build continues.

The versioned image build keeps the upstream Podman, Buildah, and Skopeo CLI
checks. BuildKit cannot create the nested namespace needed by the upstream
`podman networking` assertion, so setup marks only that assertion skipped in
the staged `/imagegeneration/tests` copy; the pinned upstream source remains
unchanged. Podman is still `provided`: the generated compatibility command
creates, lists, and removes a uniquely named bridge network. Local Docker
conformance runs that command in the existing privileged executor, while
Sandbox conformance runs it in the actual Qiniu template runtime.

## Source of truth

- [`runner-images-upstream.lock.json`](runner-images-upstream.lock.json) pins the
  `actions/runner-images` revision, report paths, and report checksums.
- Each template Dockerfile pins the Canonical-published Ubuntu OCI index from
  `public.ecr.aws/ubuntu/ubuntu` by digest. Canonical publishes the same Ubuntu
  images through its documented OCI registry endpoints; the ECR endpoint is
  reachable by Sandbox builders in both supported regions. The template matrix
  check rejects a tag-only reference or a different registry.
- [`runner-images-compatibility.json`](runner-images-compatibility.json) is the
  machine-readable coverage and conformance input. It is the only full
  inventory in this repository.
- Each template README records build input, resources, and runner behavior.
- Each `software-diff.md` records image-specific exclusions and Sandbox
  service differences without duplicating the inventory.

## Build and verification

Qiniu Sandbox templates are officially built and published with
`qiniu/qshell` 2.19.10 or newer.
Docker builds are local conformance inputs only: a successful Docker build does
not create, rebuild, or publish a Qiniu Sandbox template.

The build and verification commands are:

```bash
task template-check-all
task template-build-ubuntu-slim
task template-build-ubuntu-22-04
task template-build-ubuntu-24-04
task template-build-ubuntu-26-04
task template-conformance-local
task template-smoke IMAGE_KEY=ubuntu-24.04 TEMPLATE_ID=<published-template-id>
```

The formal template gate is a qshell build reaching terminal `Status: ready`,
followed by release smoke inside a real Sandbox created from that template.
Release smoke checks the OS, architecture, preinstalled Actions runner,
outbound HTTPS, Docker, writable work/tool-cache paths, and cleanup. Full
per-inventory runtime conformance and local Docker builds remain optional
diagnostics; neither is a substitute for the remote usability gate.

Each `template-build-*` target must:

1. copy the tracked template's `qshell.sandbox.toml` to a temporary file;
2. keep the working directory at that template directory so the relative
   Dockerfile and build context continue to resolve;
3. run `qshell sandbox template build --wait --config <temporary-file>`; and
4. remove the temporary file on exit.

The temporary copy is required because a first Qiniu template creation writes
the resulting `template_id` into the configuration file. The tracked
`qshell.sandbox.toml` remains a stable, reviewable input and must not receive
that environment-specific identifier.

The underlying Task 7 gates can also be run directly:

```bash
bash scripts/check-runner-template-matrix.sh
bash scripts/check-runner-image-compatibility.sh
```

## Maintenance and promotion

Update the upstream lock in a reviewed change, regenerate the compatibility
manifest from all four pinned reports, inspect every changed
`provided`/`excluded` decision, and rebuild all physical templates. The runner
platform owner is responsible for the lock and compatibility review; the Qiniu
Sandbox operator owns regional publication.

Promotion requires successful release smoke in both supported regions. Attach
the JSON evidence to Issue #38 before moving a row to `verified`. Rollback
restores the previous public template ID/name mapping and the previous reviewed
lock/manifest together, then repeats both-region smoke.
Task 8 exposes matching `task template-publish-*` and
`task template-unpublish-*` commands for the four physical templates; use
`task template-defaults-check` before promotion and the matching unpublish
target for rollback. Qshell publish and unpublish do not support the build-only
`--config` option. Their Task targets run from the matching template directory,
read the stable name from its tracked `qshell.sandbox.toml`, and invoke the
matching operation with `-y`; no Region-specific template ID is committed.

## Current versus target state

All rows above are currently `development`. They are tracked build definitions,
not a claim that the templates are public. The target catalog becomes public
only through the publication and verification transitions described above.
