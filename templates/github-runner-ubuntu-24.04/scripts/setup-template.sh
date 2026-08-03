#!/usr/bin/env bash
set -euxo pipefail

: "${RUNNER_IMAGES_REV:?RUNNER_IMAGES_REV is required}"
: "${RUNNER_IMAGES_ARCHIVE_SHA256:?RUNNER_IMAGES_ARCHIVE_SHA256 is required}"
: "${RUNNER_VERSION:?RUNNER_VERSION is required}"
: "${RUNNER_ARCHIVE_SHA256:?RUNNER_ARCHIVE_SHA256 is required}"
: "${DOCKER_GPG_SHA256:?DOCKER_GPG_SHA256 is required}"
: "${DOCKER_GPG_FINGERPRINT:?DOCKER_GPG_FINGERPRINT is required}"
export PATH="/usr/local/share/qiniu-sandbox-runner-template:${PATH}"

download_checked() {
  local url="$1"
  local destination="$2"
  local expected_sha256="$3"
  curl --http1.1 -fsSL --connect-timeout 15 --max-time 1800 \
    --retry 5 --retry-all-errors --retry-delay 2 \
    "$url" -o "$destination"
  echo "$expected_sha256  $destination" | sha256sum --check -
}

configure_reliable_apt_sources() {
  cat >/etc/apt/apt-mirrors.txt <<'APT_MIRRORS'
https://mirrors.tuna.tsinghua.edu.cn/ubuntu/	priority:1
https://mirrors.edge.kernel.org/ubuntu/	priority:2
https://archive.ubuntu.com/ubuntu/	priority:3
APT_MIRRORS
  local source_file
  for source_file in /etc/apt/sources.list /etc/apt/sources.list.d/ubuntu.sources; do
    [ -f "$source_file" ] || continue
    sed -i \
      -e 's|http://azure.archive.ubuntu.com/ubuntu|mirror+file:/etc/apt/apt-mirrors.txt|g' \
      -e 's|http://archive.ubuntu.com/ubuntu|mirror+file:/etc/apt/apt-mirrors.txt|g' \
      -e 's|https://archive.ubuntu.com/ubuntu|mirror+file:/etc/apt/apt-mirrors.txt|g' \
      -e 's|http://security.ubuntu.com/ubuntu|mirror+file:/etc/apt/apt-mirrors.txt|g' \
      -e 's|https://security.ubuntu.com/ubuntu|mirror+file:/etc/apt/apt-mirrors.txt|g' \
      "$source_file"
  done
  cat >/etc/apt/apt.conf.d/80qiniu-network <<'APT_NETWORK'
Acquire::Retries "5";
Acquire::http::Timeout "30";
Acquire::https::Timeout "30";
APT_NETWORK
}

ensure_upstream_apt_source_layout() {
  # Canonical's ECR rootfs remains apt-functional through sources.list, while
  # runner-images' Ubuntu 24 setup unconditionally rewrites the deb822 path
  # used by its Azure image. Keep the active source and provide that path.
  install -d -m 0755 /etc/apt/sources.list.d
  if [ ! -e /etc/apt/sources.list.d/ubuntu.sources ]; then
    install -m 0644 /dev/null /etc/apt/sources.list.d/ubuntu.sources
  fi
}

install_docker_for_sandbox() {
  install -d -m 0755 /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /tmp/docker.gpg
  echo "${DOCKER_GPG_SHA256}  /tmp/docker.gpg" | sha256sum --check -
  actual_fingerprint="$(gpg --show-keys --with-colons /tmp/docker.gpg | awk -F: '$1 == "fpr" {print $10; exit}')"
  test "$actual_fingerprint" = "$DOCKER_GPG_FINGERPRINT"
  gpg --dearmor -o /etc/apt/keyrings/docker.gpg /tmp/docker.gpg
  chmod a+r /etc/apt/keyrings/docker.gpg
  . /etc/os-release
  echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
    >/etc/apt/sources.list.d/docker.list
  apt-get update
  if apt-cache show docker-ce >/dev/null 2>&1; then
    apt-get install -y --no-install-recommends \
      containerd.io docker-buildx-plugin docker-ce docker-ce-cli docker-compose-plugin
  else
    apt-get install -y --no-install-recommends docker.io docker-buildx docker-compose-v2
  fi
}

install_runner() {
  install -d -m 0755 /opt/actions-runner
  download_checked \
    "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" \
    /tmp/actions-runner.tar.gz \
    "$RUNNER_ARCHIVE_SHA256"
  tar -xzf /tmp/actions-runner.tar.gz -C /opt/actions-runner
  /opt/actions-runner/bin/installdependencies.sh
  test -x /opt/actions-runner/config.sh
  test -x /opt/actions-runner/run.sh
}

stop_validated_service() {
  local unit="$1"
  if [ "$unit" = apache2 ]; then
    apache2ctl stop || true
    for _ in $(seq 1 100); do
      if ! ss -ltn 'sport = :80' | grep -q LISTEN; then
        return 0
      fi
      sleep 0.1
    done
    echo "validated service kept port 80 busy after cleanup: apache2" >&2
    ss -ltnp 'sport = :80' >&2 || true
    return 1
  fi
  systemctl stop "$unit" || true
  if systemctl is-active --quiet "$unit"; then
    echo "validated service remained active after cleanup: $unit" >&2
    return 1
  fi
}

apt-get update
apt-get install -y --no-install-recommends ca-certificates
configure_reliable_apt_sources
apt-get update
# The pinned runner-images toolset asks apt for the ambiguous virtual netcat
# package. Install its concrete provider before the upstream installer and
# Pester check run.
apt-get install -y --no-install-recommends \
  curl gpg jq lsb-release man-db netcat-openbsd sudo tar wget xz-utils

if ! id -u runner >/dev/null 2>&1; then
  useradd --create-home --shell /bin/bash runner
fi
usermod -aG sudo runner
echo "runner ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/90-runner
chmod 0440 /etc/sudoers.d/90-runner
install -d -o runner -g runner \
  /home/runner/.config/git \
  /home/runner/actions-runner \
  /home/runner/work \
  /home/runner/_runnerd-hooks \
  /opt/actions-runner

download_checked \
  "https://codeload.github.com/actions/runner-images/tar.gz/${RUNNER_IMAGES_REV}" \
  /tmp/runner-images.tar.gz \
  "$RUNNER_IMAGES_ARCHIVE_SHA256"
mkdir -p /tmp/runner-images
tar -xzf /tmp/runner-images.tar.gz -C /tmp/runner-images --strip-components=1

export DEBIAN_FRONTEND=noninteractive
export HELPER_SCRIPTS=/tmp/runner-images/images/ubuntu-slim/scripts/helpers
export INSTALLER_SCRIPT_FOLDER=/tmp/runner-images/images/ubuntu-slim/toolsets
export IMAGE_VERSION="${IMAGE_VERSION:-${ImageVersion:-local}}"
export IMAGE_OS=ubuntu24

if [ "${TEMPLATE_FLAVOR:-}" = slim ]; then
  upstream_build=/tmp/runner-images/images/ubuntu-slim/scripts/build
  for installer in \
    configure-apt-sources.sh \
    configure-apt.sh \
    install-apt-vital.sh \
    install-ms-repos.sh \
    configure-image-data-file.sh \
    configure-environment.sh \
    install-actions-cache.sh \
    install-apt-common.sh \
    install-azcopy.sh \
    install-azure-cli.sh \
    install-azure-devops-cli.sh \
    install-bicep.sh \
    install-aws-tools.sh \
    install-git.sh \
    install-git-lfs.sh \
    install-github-cli.sh \
    install-google-cloud-cli.sh \
    install-nvm.sh \
    install-nodejs.sh \
    install-powershell.sh \
    configure-dpkg.sh \
    install-yq.sh \
    install-python.sh \
    install-zstd.sh \
    install-pipx-packages.sh \
    install-docker-cli.sh \
    configure-system.sh; do
    bash "$upstream_build/$installer"
  done
  ln -s /etc/skel/.nvm /home/runner/.nvm
else
  . /etc/os-release
  export IMAGE_OS="ubuntu${VERSION_ID/.}"
  install -d -m 0755 /tmp/qiniu-runner-build-tools
  cat >/tmp/qiniu-runner-build-tools/systemctl <<'SYSTEMCTL'
#!/bin/sh
action=${1:-}
if [ "$action" = --version ]; then
  exec /usr/bin/systemctl "$@"
fi
shift || true
if [ "$action" = is-active ] && [ "${1:-}" = --quiet ]; then
  shift
fi
unit=${1:-}
unit=${unit%.service}
run_isolated() {
  /usr/bin/python3 - "$@" <<'PYTHON'
import subprocess
import sys

try:
    result = subprocess.run(sys.argv[1:], stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, close_fds=True, timeout=30)
except subprocess.TimeoutExpired:
    sys.exit(124)
sys.exit(result.returncode)
PYTHON
}
run_detached_until_pid_state() {
  desired_state="$1"
  pid_file="$2"
  shift 2
  /usr/bin/python3 - "$desired_state" "$pid_file" "$@" <<'PYTHON'
import os
import signal
import subprocess
import sys
import time

desired_active = sys.argv[1] == "active"
pid_file = sys.argv[2]
process = subprocess.Popen(
    sys.argv[3:],
    stdin=subprocess.DEVNULL,
    stdout=subprocess.DEVNULL,
    stderr=subprocess.DEVNULL,
    close_fds=True,
    start_new_session=True,
)

def service_is_active():
    try:
        with open(pid_file, encoding="utf-8") as stream:
            pid = int(stream.read().strip())
        os.kill(pid, 0)
        return True
    except (FileNotFoundError, ProcessLookupError, ValueError):
        return False

deadline = time.monotonic() + 30
while time.monotonic() < deadline:
    if service_is_active() == desired_active:
        sys.exit(0)
    return_code = process.poll()
    if return_code not in (None, 0):
        sys.exit(return_code)
    time.sleep(0.1)

if process.poll() is None:
    os.killpg(process.pid, signal.SIGTERM)
sys.exit(124)
PYTHON
}
start_apache() {
  run_detached_until_pid_state active /run/apache2/apache2.pid /usr/sbin/apachectl start
}
stop_apache() {
  run_detached_until_pid_state inactive /run/apache2/apache2.pid /usr/sbin/apachectl stop
}
case "$unit:$action" in
  apache2:start)
    start_apache
    exit $?
    ;;
  apache2:stop)
    stop_apache
    exit $?
    ;;
  apache2:restart)
    stop_apache || exit $?
    start_apache
    exit $?
    ;;
  apache2:is-active)
    test -s /run/apache2/apache2.pid && kill -0 "$(cat /run/apache2/apache2.pid)" 2>/dev/null
    exit $?
    ;;
  nginx:start)
    run_isolated /usr/sbin/nginx
    exit $?
    ;;
  nginx:stop)
    run_isolated /usr/sbin/nginx -s quit
    exit $?
    ;;
  nginx:restart)
    if test -s /run/nginx.pid && kill -0 "$(cat /run/nginx.pid)" 2>/dev/null; then
      run_isolated /usr/sbin/nginx -s reload
      exit $?
    fi
    run_isolated /usr/sbin/nginx
    exit $?
    ;;
  nginx:is-active)
    test -s /run/nginx.pid && kill -0 "$(cat /run/nginx.pid)" 2>/dev/null
    exit $?
    ;;
esac
# VM-oriented upstream installers assume service operations always return. Bound
# SysV calls so a missing init environment cannot wedge a Sandbox build forever.
# Isolate their file descriptors so a daemon cannot keep Pester's capture pipe
# open after the service command exits or is killed.
service_status() {
  run_isolated /usr/sbin/service "$unit" status
}
case "$action" in
  start|stop|restart)
    if [ -x "/etc/init.d/$unit" ]; then
      run_isolated /usr/sbin/service "$unit" "$action"
      service_result=$?
      if [ "$service_result" -eq 0 ]; then
        exit 0
      fi
      if service_status; then
        [ "$action" = stop ] && exit "$service_result"
        exit 0
      fi
      [ "$action" = stop ] && exit 0
      exit "$service_result"
    fi
    ;;
  is-active)
    if [ -x "/etc/init.d/$unit" ]; then
      service_status
      exit $?
    fi
    ;;
esac
echo "qiniu runner template build: skipping VM-only systemctl $action $*" >&2
exit 0
SYSTEMCTL
  chmod 0755 /tmp/qiniu-runner-build-tools/systemctl
  ln -s /tmp/qiniu-runner-build-tools/systemctl /usr/local/bin/systemctl
  export PATH="/tmp/qiniu-runner-build-tools:$PATH"
  echo 'APT::Get::Assume-Yes "true";' >/etc/apt/apt.conf.d/90assumeyes
  install -d -m 0755 /etc/cloud/templates
  cat >/etc/waagent.conf <<'WAAGENT'
ResourceDisk.Format=n
ResourceDisk.EnableSwap=n
ResourceDisk.SwapSizeMB=0
WAAGENT
  release_digits="${VERSION_ID/.}"
  upstream_build=/tmp/runner-images/images/ubuntu/scripts/build
  export HELPER_SCRIPTS=/tmp/runner-images/images/ubuntu/scripts/helpers
  export INSTALLER_SCRIPT_FOLDER=/tmp/runner-images/images/ubuntu/scripts/build
  cp "/tmp/runner-images/images/ubuntu/toolsets/toolset-${release_digits}.json" \
    "$INSTALLER_SCRIPT_FOLDER/toolset.json"
  install -d -m 0755 /imagegeneration
  cp -a "$HELPER_SCRIPTS" /imagegeneration/helpers
  cp -a "$HELPER_SCRIPTS/../tests" /imagegeneration/tests
  # BuildKit denies the namespace clone used by this one runtime assertion.
  # Keep the three CLI checks here and run the network lifecycle in conformance.
  podman_networking_test=/imagegeneration/tests/Tools.Tests.ps1
  test "$(grep -Fxc '    It "podman networking" -TestCases "podman CNI plugins" {' "$podman_networking_test" || true)" -eq 1
  sed -i 's/    It "podman networking" -TestCases "podman CNI plugins" {/    It "podman networking" -Skip -TestCases "podman CNI plugins" {/' "$podman_networking_test"
  test "$(grep -Fxc '    It "podman networking" -Skip -TestCases "podman CNI plugins" {' "$podman_networking_test" || true)" -eq 1
  test "$(grep -Fxc '    $testCases = @("podman", "buildah", "skopeo") | ForEach-Object { @{ContainerCommand = $_} }' "$podman_networking_test" || true)" -eq 1
  test "$(grep -Fxc '    It "<ContainerCommand>" -TestCases $testCases {' "$podman_networking_test" || true)" -eq 1
  test "$(grep -Fxc '        "$ContainerCommand -v" | Should -ReturnZeroExitCode' "$podman_networking_test" || true)" -eq 1

  bash "$upstream_build/install-ms-repos.sh"
  ensure_upstream_apt_source_layout
  bash "$upstream_build/configure-apt-sources.sh"
  configure_reliable_apt_sources
  bash "$upstream_build/configure-apt.sh"
  bash "$upstream_build/configure-environment.sh"
  bash "$upstream_build/install-apt-vital.sh"
  bash "$upstream_build/install-powershell.sh"
  pwsh -File "$upstream_build/Install-PowerShellModules.ps1"
  pwsh -File "$upstream_build/Install-PowerShellAzModules.ps1"

  for installer in \
    install-actions-cache.sh \
    install-apt-common.sh \
    install-azcopy.sh \
    install-azure-cli.sh \
    install-azure-devops-cli.sh \
    install-bicep.sh \
    install-apache.sh \
    install-aws-tools.sh \
    install-clang.sh \
    install-swift.sh \
    install-cmake.sh \
    install-codeql-bundle.sh \
    install-awf.sh \
    install-container-tools.sh \
    install-dotnetcore-sdk.sh \
    install-microsoft-edge.sh \
    install-gcc-compilers.sh \
    install-firefox.sh \
    install-gfortran.sh \
    install-git.sh \
    install-git-lfs.sh \
    install-github-cli.sh \
    install-google-chrome.sh \
    install-google-cloud-cli.sh \
    install-haskell.sh \
    install-java-tools.sh \
    install-kubernetes-tools.sh \
    install-miniconda.sh \
    install-kotlin.sh \
    install-mysql.sh \
    install-nginx.sh \
    install-nvm.sh \
    install-nodejs.sh \
    install-copilot-cli.sh \
    install-bazel.sh \
    install-php.sh \
    install-postgresql.sh \
    install-pulumi.sh \
    install-ruby.sh \
    install-rust.sh \
    install-julia.sh \
    install-selenium.sh \
    install-packer.sh \
    install-vcpkg.sh \
    configure-dpkg.sh \
    install-yq.sh \
    install-android-sdk.sh \
    install-pypy.sh \
    install-python.sh \
    install-zstd.sh \
    install-ninja.sh; do
    bash "$upstream_build/$installer"
    case "$installer" in
      install-apache.sh) stop_validated_service apache2 ;;
      install-mysql.sh) stop_validated_service mysql ;;
      install-nginx.sh) stop_validated_service nginx ;;
      install-postgresql.sh) stop_validated_service postgresql ;;
    esac
  done

  if [ "$VERSION_ID" = 22.04 ]; then
    for installer in \
      install-aliyun-cli.sh \
      install-heroku.sh \
      install-leiningen.sh \
      install-mssql-tools.sh \
      install-oc-cli.sh \
      install-oras-cli.sh \
      install-rlang.sh \
      install-mono.sh \
      install-sbt.sh \
      install-sqlpackage.sh \
      install-terraform.sh; do
      if [ "$installer" = install-mono.sh ]; then
        # Mono 6.12 JIT aborts when an amd64 image is built through arm64
        # emulation. Scope interpreter mode to installation-time validation;
        # the completed amd64 image retains the native JIT default.
        MONO_ENV_OPTIONS=--interp bash "$upstream_build/$installer"
      else
        bash "$upstream_build/$installer"
      fi
    done
  fi

  pwsh -File "$upstream_build/Install-Toolset.ps1"
  pwsh -File "$upstream_build/Configure-Toolset.ps1"
  . "$HELPER_SCRIPTS/etc-environment.sh"
  reload_etc_environment
  bash "$upstream_build/install-pipx-packages.sh"
  sudo -H -u runner \
    HELPER_SCRIPTS="$HELPER_SCRIPTS" \
    INSTALLER_SCRIPT_FOLDER="$INSTALLER_SCRIPT_FOLDER" \
    bash "$upstream_build/install-homebrew.sh"
fi

install_docker_for_sandbox
install_runner
install -m 0755 \
  /usr/local/share/qiniu-sandbox-runner-template/ensure-docker \
  /usr/local/bin/ensure-docker
usermod -aG docker runner
chown -R runner:runner \
  /home/runner \
  /opt/actions-runner \
  /opt/hostedtoolcache
chmod -R a+rX /opt/actions-runner /opt/hostedtoolcache

apt-get clean
find /var/lib/apt/lists -mindepth 1 -delete
find /tmp/runner-images -mindepth 1 -delete
find /tmp/runner-images -depth -type d -empty -delete
if [ -d /imagegeneration ]; then
  find /imagegeneration -mindepth 1 -delete
  rmdir /imagegeneration
fi
rm -f /usr/local/bin/systemctl
find /tmp/qiniu-runner-build-tools -mindepth 1 -delete 2>/dev/null || true
rmdir /tmp/qiniu-runner-build-tools 2>/dev/null || true
rm -f /tmp/actions-runner.tar.gz /tmp/docker.gpg /tmp/runner-images.tar.gz
