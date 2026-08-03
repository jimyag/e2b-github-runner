#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"
templates_readme="${RUNNER_TEMPLATES_README:-templates/README.md}"

fail() {
  echo "runner template matrix: $*" >&2
  exit 1
}

test -f "$templates_readme" || fail "missing templates README $templates_readme"

expected_name() {
  case "$1" in
    ubuntu-slim) echo github-runner-ubuntu-slim ;;
    ubuntu-22.04) echo github-runner-ubuntu-22-04 ;;
    ubuntu-24.04) echo github-runner-ubuntu-24-04 ;;
    ubuntu-26.04) echo github-runner-ubuntu-26-04 ;;
    *) return 1 ;;
  esac
}

expected_base_reference() {
  case "$1" in
    ubuntu-slim | ubuntu-24.04)
      echo public.ecr.aws/ubuntu/ubuntu:24.04@sha256:be20a0347f238b7d373edddc55923443b21dd9a60277bf8a93e43458cd0bf2fc
      ;;
    ubuntu-22.04)
      echo public.ecr.aws/ubuntu/ubuntu:22.04@sha256:0bc16241504efa4cf92fcc8c8039dac604c6bb832b3d8fcc24097c6b05b60b7c
      ;;
    ubuntu-26.04)
      echo public.ecr.aws/ubuntu/ubuntu:26.04@sha256:a5da6f6b18c3a4b8dcc73244592f7096f417d2667966d0e33460e9e308f25f67
      ;;
    *) return 1 ;;
  esac
}

expected_build_target() {
  case "$1" in
    ubuntu-slim) echo template-build-ubuntu-slim ;;
    ubuntu-22.04) echo template-build-ubuntu-22-04 ;;
    ubuntu-24.04) echo template-build-ubuntu-24-04 ;;
    ubuntu-26.04) echo template-build-ubuntu-26-04 ;;
    *) return 1 ;;
  esac
}

seen_names_file="$(mktemp)"
cleanup() {
  find "$seen_names_file" -type f -delete 2>/dev/null || true
}
trap cleanup EXIT
for image_key in ubuntu-slim ubuntu-22.04 ubuntu-24.04 ubuntu-26.04; do
  directory="templates/github-runner-${image_key}"
  test -d "$directory" || fail "missing directory $directory"
  for required_file in Dockerfile qshell.sandbox.toml README.md software-diff.md scripts/setup-template.sh scripts/ensure-docker; do
    test -f "$directory/$required_file" || fail "missing $directory/$required_file"
  done

  template_name="$(
    awk -F= '/^[[:space:]]*name[[:space:]]*=/ {
      value=$2
      gsub(/[[:space:]"]/, "", value)
      print value
      exit
    }' "$directory/qshell.sandbox.toml"
  )"
  wanted_name="$(expected_name "$image_key")"
  test "$template_name" = "$wanted_name" ||
    fail "$image_key template name is $template_name, want $wanted_name"
  if grep -Fxq "$template_name" "$seen_names_file"; then
    fail "duplicate physical template name $template_name"
  fi
  echo "$template_name" >>"$seen_names_file"

  cpu_count="$(awk -F= '/^[[:space:]]*cpu_count[[:space:]]*=/ {gsub(/[[:space:]]/, "", $2); print $2; exit}' "$directory/qshell.sandbox.toml")"
  memory_mb="$(awk -F= '/^[[:space:]]*memory_mb[[:space:]]*=/ {gsub(/[[:space:]]/, "", $2); print $2; exit}' "$directory/qshell.sandbox.toml")"
  test "$cpu_count" = 8 || fail "$image_key cpu_count is $cpu_count, want 8"
  test "$memory_mb" = 8192 || fail "$image_key memory_mb is $memory_mb, want 8192"

  base_reference="$(
    awk '
      toupper($1) == "FROM" {
        for (field_index = 1; field_index <= NF; field_index++) {
          if ($field_index ~ /^public\.ecr\.aws\/ubuntu\/ubuntu:[0-9][0-9]\.[0-9][0-9]@sha256:[0-9a-f]{64}$/) {
            print $field_index
            exit
          }
        }
      }' "$directory/Dockerfile"
  )"
  wanted_base_reference="$(expected_base_reference "$image_key")"
  test "$base_reference" = "$wanted_base_reference" ||
    fail "$image_key base reference is $base_reference, want $wanted_base_reference"

  if grep -Eq '^[[:space:]]*RUN[[:space:]]+--mount=|/run/secrets/github_token' "$directory/Dockerfile"; then
    fail "$image_key uses a BuildKit-only RUN secret that qshell v2 cannot execute"
  fi
  grep -Eq '^[[:space:]]*RUN[[:space:]]+TEMPLATE_FLAVOR=' "$directory/Dockerfile" ||
    fail "$image_key setup must use a plain qshell-compatible RUN instruction"
  if grep -Fq 'Acquire::https::Verify-Peer=false' "$directory/scripts/setup-template.sh"; then
    fail "$image_key setup must not disable apt HTTPS peer verification"
  fi
  grep -Fq 'MONO_ENV_OPTIONS=--interp bash "$upstream_build/$installer"' \
    "$directory/scripts/setup-template.sh" ||
    fail "$image_key must run the legacy Mono installer in interpreter mode for amd64 emulation"
  if [ "$image_key" = ubuntu-22.04 ]; then
    grep -Fq 'dotnet tool install Microsoft.SqlPackage' \
      "$directory/scripts/setup-template.sh" ||
      fail "$image_key must install SqlPackage through the official NuGet tool feed"
    grep -Fq -- '--version "$SQLPACKAGE_DOTNET_TOOL_VERSION"' \
      "$directory/scripts/setup-template.sh" ||
      fail "$image_key must pin the SqlPackage NuGet tool version"
    grep -Fq 'bash "$HELPER_SCRIPTS/invoke-tests.sh" "Tools" "SqlPackage"' \
      "$directory/scripts/setup-template.sh" ||
      fail "$image_key must run the pinned upstream SqlPackage test"
  fi

  build_target="$(expected_build_target "$image_key")"
  grep -Fq "task $build_target" "$directory/README.md" ||
    fail "$image_key README must use the exact task $build_target build command"
done

if grep -En 'runner-template-build-all|qshell sandbox template (publish|unpublish).*--config' \
  "$templates_readme" templates/github-runner-*/README.md >/dev/null; then
  fail "template docs reference an unsupported aggregate target or publish/unpublish --config"
fi
grep -Fq 'Qshell publish and unpublish do not support the build-only' "$templates_readme" ||
  fail "templates README must state that publish/unpublish do not support build-only --config"
grep -Fq 'read the stable name from its tracked `qshell.sandbox.toml`' "$templates_readme" ||
  fail "templates README must use the tracked stable name for publish/unpublish"

readme_catalog="$(
  awk -F'|' '
    /^\| `ubuntu-/ {
      for (field_index = 2; field_index <= 5; field_index++) {
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", $field_index)
        gsub(/`/, "", $field_index)
      }
      print $2 "\t" $3 "\t" $4 "\t" $5
    }' "$templates_readme"
)"
expected_catalog=$'ubuntu-slim\tgithub-runner-ubuntu-slim\tUbuntu Slim x64\tstable\nubuntu-22.04\tgithub-runner-ubuntu-22-04\tUbuntu 22.04 x64\tfollows upstream deprecation\nubuntu-24.04\tgithub-runner-ubuntu-24-04\tUbuntu 24.04 x64\tstable\nubuntu-26.04\tgithub-runner-ubuntu-26-04\tUbuntu 26.04 x64\tpreview\nubuntu-latest\tgithub-runner-ubuntu-24-04\tUbuntu 24.04 x64\tstable logical mapping'
test "$readme_catalog" = "$expected_catalog" ||
  fail "templates/README.md support matrix does not match the five public logical rows"

publication_states="$(
  awk -F'|' '
    /^\| `ubuntu-/ {
      label=$2
      state=$6
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", label)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", state)
      gsub(/`/, "", label)
      print label "\t" state
    }' "$templates_readme"
)"
invalid_states="$(
  awk -F'\t' '$2 != "development" && $2 != "published" && $2 != "verified" {print $1 "=" $2}' \
    <<<"$publication_states"
)"
test -z "$invalid_states" ||
  fail "unsupported publication state: $invalid_states"
state_24="$(awk -F'\t' '$1 == "ubuntu-24.04" {print $2}' <<<"$publication_states")"
state_latest="$(awk -F'\t' '$1 == "ubuntu-latest" {print $2}' <<<"$publication_states")"
test -n "$state_24" && test "$state_latest" = "$state_24" ||
  fail "ubuntu-latest publication state must match ubuntu-24.04"

if find templates -mindepth 1 -maxdepth 1 -type d -name '*ubuntu-latest*' | grep -q .; then
  fail "ubuntu-latest must not have a fifth physical template directory"
fi

echo "runner template matrix: 4 physical templates and 5 logical mappings verified"
