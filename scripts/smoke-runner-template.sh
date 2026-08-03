#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: scripts/smoke-runner-template.sh IMAGE_KEY TEMPLATE_ID" >&2
  exit 64
fi

image_key="$1"
template_id="$2"
repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest_file="${RUNNER_IMAGES_MANIFEST:-$repository_root/templates/runner-images-compatibility.json}"
workdir="$(mktemp -d)"
augmented_manifest="$workdir/compatibility.json"
output_dir="${RUNNER_SMOKE_OUTPUT_DIR:-$repository_root/.build/runner-conformance}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
output="$output_dir/${image_key}-${timestamp}.json"
qshell_bin="${QSHELL:-qshell}"

cleanup() {
  find "$workdir" -type f -delete 2>/dev/null || true
  rmdir "$workdir" 2>/dev/null || true
}
trap cleanup EXIT

command -v "$qshell_bin" >/dev/null 2>&1 || {
  echo "qshell executable not found: $qshell_bin" >&2
  exit 69
}
qshell_version="$("$qshell_bin" version 2>&1 | sed -nE 's/^v([0-9]+\.[0-9]+\.[0-9]+).*$/\1/p' | head -n 1)"
test -n "$qshell_version" || {
  echo "could not determine qshell version" >&2
  exit 69
}
if ! awk -v actual="$qshell_version" -v minimum="2.19.10" '
  BEGIN {
    split(actual, a, ".")
    split(minimum, m, ".")
    for (i = 1; i <= 3; i++) {
      if ((a[i] + 0) > (m[i] + 0)) exit 0
      if ((a[i] + 0) < (m[i] + 0)) exit 1
    }
    exit 0
  }
'; then
  echo "qshell >= 2.19.10 is required; found v$qshell_version" >&2
  exit 69
fi

case "$image_key" in
  ubuntu-slim | ubuntu-24.04)
    expected_release=24.04
    support_channel=stable
    ;;
  ubuntu-22.04)
    expected_release=22.04
    support_channel=stable
    ;;
  ubuntu-26.04)
    expected_release=26.04
    support_channel=preview
    ;;
  *)
    echo "unknown image key $image_key" >&2
    exit 65
    ;;
esac

jq \
  --arg image "$image_key" \
  --arg expected_release "$expected_release" \
  '
    .images[$image].entries = [
      {
        category: "Release smoke",
        upstream_name: "OS release",
        status: "provided",
        verification: (". /etc/os-release; test \"$VERSION_ID\" = \"" + $expected_release + "\"")
      },
      {
        category: "Release smoke",
        upstream_name: "x86_64 architecture",
        status: "provided",
        verification: "test \"$(uname -m)\" = x86_64"
      },
      {
        category: "Release smoke",
        upstream_name: "preinstalled Actions runner",
        status: "provided",
        verification: "test -x /opt/actions-runner/config.sh && test -x /opt/actions-runner/run.sh"
      },
      {
        category: "Release smoke",
        upstream_name: "outbound HTTPS",
        status: "provided",
        verification: "curl -fsS --connect-timeout 15 https://api.github.com/zen >/dev/null"
      },
      {
        category: "Release smoke",
        upstream_name: "Docker daemon",
        status: "provided",
        verification: "sudo -H -u runner -- bash -lc \"ensure-docker && docker info >/dev/null && docker run --rm hello-world >/dev/null\""
      },
      {
        category: "Release smoke",
        upstream_name: "runner writable work and tool-cache paths",
        status: "provided",
        verification: "test -d /home/runner/work && test -w /home/runner/work && test -w /opt/hostedtoolcache"
      }
    ]
  ' "$manifest_file" >"$augmented_manifest"

mkdir -p "$output_dir"
RUNNER_IMAGES_MANIFEST="$augmented_manifest" \
  QSHELL="$qshell_bin" \
  "$repository_root/scripts/run-runner-image-conformance.sh" \
  --image "$image_key" \
  --executor sandbox \
  --target "$template_id" \
  --output "$output"

jq --arg support_channel "$support_channel" \
  '.support_channel = $support_channel' \
  "$output" >"$workdir/result.json"
mv "$workdir/result.json" "$output"

echo "$output"
