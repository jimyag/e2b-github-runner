# Managed runner labels

Use a supported Qiniu label pair to select a maintained public Sandbox template without creating a custom Runner Spec.

## Supported labels

| Workflow request | Template status | Notes |
| --- | --- | --- |
| `[qiniu, ubuntu-slim]` | Stable | Smaller general-purpose image |
| `[qiniu, ubuntu-22.04]` | Stable | Ubuntu 22.04 x64 |
| `[qiniu, ubuntu-24.04]` | Stable | Recommended default |
| `[qiniu, ubuntu-26.04]` | Preview | Preview image, use deliberately |
| `[qiniu, ubuntu-latest]` | Stable mapping | Currently maps to Ubuntu 24.04 |

## Matching contract

Each managed spec advertises `self-hosted`, `linux`, `x64`, `qiniu`, and its exact OS label. It requires `qiniu` plus that OS label.

Matching preserves:

```text
required labels ⊆ job labels ⊆ advertised labels
```

This means `[qiniu, ubuntu-24.04]` and the full advertised set both match. Partial or unsupported sets do not.

## Managed and custom ownership

runnerd owns managed names, labels, required labels, public template names, priority, and default availability. Operators control `enabled`, `max_concurrency`, and `min_idle`.

Custom specs remain operator-owned. They use an explicit template ID and may define different advertised and required labels. Saving a custom spec does not prove the template exists or is usable in the selected Sandbox region.

## Template resolution

Immediately before registration, runnerd resolves the managed public template name through the effective account or organization Sandbox endpoint. A stable public name can therefore resolve to different template IDs in different regions without persisting one region's ID in the spec.

See [Run your first workflow](/docs/guides/workflow) for a managed example, or [Build and use a custom runner template](/docs/guides/custom-templates) for the complete custom path.
