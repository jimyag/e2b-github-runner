#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo "usage: scripts/run-runner-template-operation.sh build|publish|unpublish TEMPLATE_DIR [BUILD_NAME]" >&2
  exit 64
fi

operation="$1"
template_dir="$2"
build_name="${3:-}"
qshell_bin="${QSHELL:-qshell}"

: "${QINIU_SANDBOX_API_URL:?QINIU_SANDBOX_API_URL is required}"
: "${QINIU_API_KEY:?QINIU_API_KEY is required}"
test -d "$template_dir" || {
  echo "template directory does not exist: $template_dir" >&2
  exit 66
}
test -f "$template_dir/qshell.sandbox.toml" || {
  echo "template config does not exist: $template_dir/qshell.sandbox.toml" >&2
  exit 66
}
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

output_file="$(mktemp)"
cleanup() {
  rm -f "$output_file"
}
trap cleanup EXIT

case "$operation" in
  build)
    (
      cd "$template_dir"
      tmp_config="$(mktemp .qshell-sandbox.XXXXXX)"
      trap 'rm -f "$tmp_config"' EXIT
      cp qshell.sandbox.toml "$tmp_config"
      build_args=(sandbox template build --wait --config "$tmp_config")
      if [ -n "$build_name" ]; then
        build_args+=(--name "$build_name")
      fi
      "$qshell_bin" "${build_args[@]}" 2>&1 | tee "$output_file"
    )
    grep -Eq '^Status:[[:space:]]+ready[[:space:]]*$' "$output_file" || {
      echo "qshell did not report terminal Status: ready" >&2
      exit 1
    }
    ;;
  publish | unpublish)
    test -z "$build_name" || {
      echo "BUILD_NAME is only valid for build" >&2
      exit 64
    }
    (
      cd "$template_dir"
      "$qshell_bin" sandbox template "$operation" -y 2>&1 | tee "$output_file"
    )
    grep -Eq "^Template .+ ${operation/publish/published}$" "$output_file" || {
      if [ "$operation" = unpublish ]; then
        grep -Eq '^Template .+ unpublished$' "$output_file" && exit 0
      fi
      echo "qshell did not confirm template $operation" >&2
      exit 1
    }
    ;;
  *)
    echo "unknown template operation: $operation" >&2
    exit 64
    ;;
esac
