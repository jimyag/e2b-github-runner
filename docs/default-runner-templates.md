# Public runner templates

[中文](zh/default-runner-templates.md)

Qiniu maintains four physical Linux x64 Sandbox templates for GitHub Actions:
Ubuntu Slim, Ubuntu 22.04, Ubuntu 24.04, and preview Ubuntu 26.04.
`ubuntu-latest` is a logical runner catalog mapping to Ubuntu 24.04, not a
fifth image.

The definitions are currently in development. This document does not claim
that a template is public until it has been built, published, catalog-checked,
and smoke-tested in both supported Sandbox regions. See
[`templates/README.md`](../templates/README.md) for pinned upstream provenance,
the compatibility contract, and per-image differences.

## Workflow labels

After both-region publication and the managed-runner rollout are verified, use
the exact pair for the requested environment:

```yaml
jobs:
  slim:
    runs-on: [qiniu, ubuntu-slim]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a

  ubuntu_22:
    runs-on: [qiniu, ubuntu-22.04]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a

  ubuntu_24:
    runs-on: [qiniu, ubuntu-24.04]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a

  ubuntu_26_preview:
    runs-on: [qiniu, ubuntu-26.04]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a

  latest:
    runs-on: [qiniu, ubuntu-latest]
    steps:
      - uses: actions/checkout@v4
      - run: uname -a
```

The `qiniu` label is mandatory. Managed matching enforces
`required_labels ⊆ job_labels ⊆ labels`, so `[ubuntu-24.04]`, `[qiniu]`, and a
request with unsupported extra labels do not match a managed default.
Operators can disable one managed spec in Admin; removing `qiniu` from a
workflow disables managed-default selection from the workflow side. Custom
specs remain available with operator-defined required labels and explicit
template IDs.

## Software compatibility

These templates track the pinned `actions/runner-images` reports item by item,
but they are not byte-for-byte GitHub-hosted runner images. The current Qiniu
Sandbox public-template build allocation exposes a 22,222-MiB root disk, while
the complete GitHub-hosted runner inventory requires more space. The three
versioned templates guarantee the Ubuntu Slim-compatible core on the requested
Ubuntu release, plus Apache, Podman, Buildah, Skopeo, Ninja, Docker support,
the preinstalled Actions runner, and the runner filesystem contract.

[`templates/runner-images-compatibility.json`](../templates/runner-images-compatibility.json)
is the executable item-level contract. `provided` means release conformance
must verify the item. `excluded` means the public template does not guarantee
it; install the tool in the workflow or build a custom Sandbox template when
it is required. An excluded executable might be present through an OS package
dependency, but workflows must not rely on it.

## Requirements

- `qiniu/qshell` 2.19.10 or newer;
- `task`, `jq`, and `curl`;
- a `QINIU_API_KEY` for the selected Sandbox region;
- `QINIU_SANDBOX_API_URL` set to that region's endpoint.

If the required qshell is not on `PATH`, pass its executable explicitly:

```bash
task template-build-ubuntu-24-04 QSHELL=/path/to/qshell
```

Every remote target fails closed when either credential variable is empty.
The tracked `qshell.sandbox.toml` files contain stable names and resource
settings only; qshell builds from temporary copies so region-specific template
IDs are never committed.

## Build and verify one region

Run the fast, credential-free source checks first:

```bash
task template-check-all
```

Then build the actual Sandbox templates with qshell. Each target waits for
qshell to report terminal `Status: ready`; a zero process exit without that
status is treated as a failed build.

```bash
task template-build-ubuntu-slim
task template-build-ubuntu-22-04
task template-build-ubuntu-24-04
task template-build-ubuntu-26-04
```

Publish only after all four builds are ready:

```bash
task template-publish-ubuntu-slim
task template-publish-ubuntu-22-04
task template-publish-ubuntu-24-04
task template-publish-ubuntu-26-04
task template-defaults-check
```

`template-defaults-check` requires exactly one public `ready` or `uploaded`
template with a nonempty ID for every stable physical name. It rejects missing
and duplicate catalog entries.

Retain each ID printed by the catalog check, then run actual Sandbox smoke:

```bash
task template-smoke IMAGE_KEY=ubuntu-slim TEMPLATE_ID=<slim-template-id>
task template-smoke IMAGE_KEY=ubuntu-22.04 TEMPLATE_ID=<22.04-template-id>
task template-smoke IMAGE_KEY=ubuntu-24.04 TEMPLATE_ID=<24.04-template-id>
task template-smoke IMAGE_KEY=ubuntu-26.04 TEMPLATE_ID=<26.04-template-id>
```

Smoke creates a temporary Sandbox with qshell and checks the OS release,
architecture, preinstalled Actions runner, outbound HTTPS, Docker daemon,
writable work/tool-cache paths, and cleanup. Preserve the emitted JSON paths as
release evidence. The full compatibility manifest remains the static inventory
contract; per-entry runtime conformance is an optional diagnostic and does not
block the release usability gate.

Local Docker builds and `task template-conformance-local` remain optional
diagnostic tools. They are not substitutes for qshell template builds or
Sandbox smoke.

## First release in both regions

Complete the whole build, publish, catalog, and smoke sequence in this order:

1. Export
   `QINIU_SANDBOX_API_URL=https://cn-yangzhou-1-sandbox.qiniuapi.com` and the
   Yangzhou `QINIU_API_KEY`.
2. Build all four templates with qshell, publish them, run
   `task template-defaults-check`, and smoke all four returned IDs.
3. Retain the build output, catalog IDs, smoke JSON, and relevant workflow URL.
4. Export
   `QINIU_SANDBOX_API_URL=https://us-south-1-sandbox.qiniuapi.com` and the
   US South `QINIU_API_KEY`.
5. Repeat the same four builds, publication, catalog check, and smoke checks.
6. Confirm both catalog results contain one runnable public entry for every
   physical stable name.
7. Attach both-region evidence to Issue #38 before marking the template rows
   verified or enabling the separate managed-runner rollout.

Qiniu owns the four physical images. `ubuntu-latest` changes only through a
reviewed runner catalog revision with new regional smoke evidence.
`ubuntu-26.04` remains preview until upstream promotes it.

## Rollback

Disable the managed Runner Specs before removing public availability. Then
run the matching reversible publication rollback:

```bash
task template-unpublish-ubuntu-slim
task template-unpublish-ubuntu-22-04
task template-unpublish-ubuntu-24-04
task template-unpublish-ubuntu-26-04
```

Do not delete template objects during an ordinary rollback. Keeping them
private preserves build history and allows a reviewed version to be
republished without changing custom Runner Specs.
