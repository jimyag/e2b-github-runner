#!/usr/bin/env bash
set -euo pipefail

runner_user="${RUNNER_USER:-runner}"
actions_runner_root="${ACTIONS_RUNNER_ROOT:-/opt/actions-runner}"
workdir="${RUNNER_WORKDIR:-/home/runner/actions-runner}"
runner_job_work="${RUNNER_JOB_WORK:-/home/runner/work}"
hook_root="${RUNNER_HOOK_ROOT:-/home/runner/_runnerd-hooks}"
ensure_docker="${ENSURE_DOCKER:-/usr/local/bin/ensure-docker}"
runner_environment_file="${RUNNER_ENVIRONMENT_FILE:-/etc/environment}"
export HOME="${RUNNER_HOME:-/home/runner}"
export XDG_CONFIG_HOME="${HOME}/.config"
export GOPATH="${GOPATH:-/opt/go}"
export GOBIN="${GOBIN:-/usr/local/bin}"
export RUNNER_TOOL_CACHE="${RUNNER_TOOL_CACHE:-/opt/hostedtoolcache}"
export AGENT_TOOLSDIRECTORY="${AGENT_TOOLSDIRECTORY:-/opt/hostedtoolcache}"
export PATH="/usr/local/go/bin:/usr/local/bin:${GOPATH}/bin:${PATH}"
if [ -r "$runner_environment_file" ]; then
  set -a
  # runner-images writes shell-compatible KEY="value" entries here.
  # The environment can contain references to build-only variables. Keep
  # nounset disabled while loading it so those entries do not abort startup.
  set +u
  # shellcheck disable=SC1090
  . "$runner_environment_file"
  set -u
  set +a
fi

if [ "$(id -u)" -eq 0 ] && id -u "$runner_user" >/dev/null 2>&1 && [ "${RUNNERD_AS_RUNNER:-}" != 1 ]; then
  install -d -o "$runner_user" -g "$runner_user" \
    "$workdir" "$runner_job_work" "$HOME" "$XDG_CONFIG_HOME/git" "$hook_root"
  exec sudo -E -u "$runner_user" \
    RUNNERD_AS_RUNNER=1 \
    RUNNER_USER="$runner_user" \
    ACTIONS_RUNNER_ROOT="$actions_runner_root" \
    RUNNER_WORKDIR="$workdir" \
    RUNNER_JOB_WORK="$runner_job_work" \
    RUNNER_HOOK_ROOT="$hook_root" \
    ENSURE_DOCKER="$ensure_docker" \
    RUNNER_ENVIRONMENT_FILE="$runner_environment_file" \
    HOME="$HOME" \
    bash "$0"
fi
if [ "$(id -u)" -eq 0 ]; then
  export RUNNER_ALLOW_RUNASROOT=1
fi

mkdir -p "$workdir" "$runner_job_work" "$HOME" "$XDG_CONFIG_HOME/git" "$hook_root"
cd "$workdir"

if [ ! -x "$actions_runner_root/config.sh" ]; then
  echo "missing preinstalled GitHub Actions runner at $actions_runner_root/config.sh" >&2
  echo "build one of the managed GitHub runner templates before starting runners" >&2
  exit 1
fi

if [ ! -x ./config.sh ]; then
  echo "copying preinstalled GitHub Actions runner"
  cp -a "$actions_runner_root"/. "$workdir"/
fi

if [ -x "$ensure_docker" ]; then
  echo "checking Docker daemon"
  "$ensure_docker"
fi

runner_url="$(printf '%%s' "%[1]s" | base64 -d)"
registration_token="$(printf '%%s' "%[2]s" | base64 -d)"
runner_name="$(printf '%%s' "%[3]s" | base64 -d)"
runner_labels="$(printf '%%s' "%[4]s" | base64 -d)"
runner_group="$(printf '%%s' "%[5]s" | base64 -d)"
runner_request_id="$(printf '%%s' "%[6]s" | base64 -d)"
sandbox_id="$(printf '%%s' "%[7]s" | base64 -d)"

export RUNNERD_SANDBOX_ID="$sandbox_id"
export RUNNERD_REQUEST_ID="$runner_request_id"
export RUNNERD_RUNNER_NAME="$runner_name"
cat >"$hook_root/job-started.sh" <<'HOOK'
#!/usr/bin/env bash
echo "RUNNERD_JOB_STARTED"
echo "::notice title=Qiniu sandbox::sandbox_id=${RUNNERD_SANDBOX_ID} runner_request_id=${RUNNERD_REQUEST_ID} runner_name=${RUNNERD_RUNNER_NAME}"
echo "Qiniu sandbox id: ${RUNNERD_SANDBOX_ID}"
echo "Runner request id: ${RUNNERD_REQUEST_ID}"
echo "Runner name: ${RUNNERD_RUNNER_NAME}"
HOOK
cat >"$hook_root/job-completed.sh" <<'HOOK'
#!/usr/bin/env bash
echo "RUNNERD_JOB_COMPLETED"
HOOK
chmod +x "$hook_root/job-started.sh" "$hook_root/job-completed.sh"
export ACTIONS_RUNNER_HOOK_JOB_STARTED="$hook_root/job-started.sh"
export ACTIONS_RUNNER_HOOK_JOB_COMPLETED="$hook_root/job-completed.sh"

config_args=(--url "$runner_url" --token "$registration_token" --name "$runner_name" --labels "$runner_labels" --work "$runner_job_work" --ephemeral --unattended --replace --disableupdate)
if [ -n "$runner_group" ]; then
  config_args+=(--runnergroup "$runner_group")
fi

echo "configuring GitHub Actions runner ${runner_name}"
retries_left=10
while [ "$retries_left" -gt 0 ]; do
  if ./config.sh "${config_args[@]}"; then
    break
  fi
  retries_left=$((retries_left - 1))
  if [ "$retries_left" -eq 0 ]; then
    echo "GitHub Actions runner configuration failed" >&2
    exit 2
  fi
  echo "GitHub Actions runner configuration failed, retrying"
  sleep 1
done
cleanup() {
  ./config.sh remove --token "$registration_token" || true
}
trap cleanup EXIT
echo "starting GitHub Actions runner"
./run.sh
