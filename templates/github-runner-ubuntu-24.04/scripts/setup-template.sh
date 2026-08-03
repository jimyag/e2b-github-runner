#!/usr/bin/env bash
set -euxo pipefail

: "${RUNNER_IMAGES_REV:?RUNNER_IMAGES_REV is required}"
: "${RUNNER_IMAGES_ARCHIVE_SHA256:?RUNNER_IMAGES_ARCHIVE_SHA256 is required}"
: "${RUNNER_VERSION:?RUNNER_VERSION is required}"
: "${RUNNER_ARCHIVE_SHA256:?RUNNER_ARCHIVE_SHA256 is required}"
: "${AZCOPY_VERSION:?AZCOPY_VERSION is required}"
: "${AZCOPY_DEB_SHA256:?AZCOPY_DEB_SHA256 is required}"
: "${AZURE_DEVOPS_EXTENSION_VERSION:?AZURE_DEVOPS_EXTENSION_VERSION is required}"
: "${AZURE_DEVOPS_EXTENSION_SHA256:?AZURE_DEVOPS_EXTENSION_SHA256 is required}"
: "${BICEP_VERSION:?BICEP_VERSION is required}"
: "${BICEP_NUGET_SHA256:?BICEP_NUGET_SHA256 is required}"
: "${GOOGLE_CLOUD_CLI_VERSION:?GOOGLE_CLOUD_CLI_VERSION is required}"
: "${GOOGLE_CLOUD_CLI_ARCHIVE_SHA256:?GOOGLE_CLOUD_CLI_ARCHIVE_SHA256 is required}"
: "${NVM_VERSION:?NVM_VERSION is required}"
: "${NVM_ARCHIVE_SHA256:?NVM_ARCHIVE_SHA256 is required}"
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

run_upstream_tests_if_available() {
  local test_script="$HELPER_SCRIPTS/invoke-tests.sh"
  if [ -f "$test_script" ]; then
    bash "$test_script" "$1" "$2"
  fi
}

install_bicep_from_nuget() {
  local package_name="azure.bicep.commandline.linux-x64.${BICEP_VERSION}.nupkg"
  local package_path="/tmp/${package_name}"
  local package_url="https://api.nuget.org/v3-flatcontainer/azure.bicep.commandline.linux-x64/${BICEP_VERSION}/${package_name}"
  local extract_dir=/tmp/qiniu-bicep

  download_checked "$package_url" "$package_path" "$BICEP_NUGET_SHA256"
  install -d -m 0755 "$extract_dir"
  unzip -q -j "$package_path" tools/bicep -d "$extract_dir"
  install -m 0755 "$extract_dir/bicep" /usr/local/bin/bicep
  find "$extract_dir" -mindepth 1 -delete
  rmdir "$extract_dir"
  rm -f "$package_path"
  bicep --version | grep -F "Bicep CLI version ${BICEP_VERSION} "
  run_upstream_tests_if_available "Tools" "Bicep"
}

install_git_lfs_from_ubuntu() {
  apt-get update
  apt-get install -y --no-install-recommends git-lfs
  git lfs version
  run_upstream_tests_if_available "Tools" "Git-lfs"
}

install_google_cloud_cli_from_archive() {
  local archive_name="google-cloud-cli-${GOOGLE_CLOUD_CLI_VERSION}-linux-x86_64.tar.gz"
  local archive_path="/tmp/${archive_name}"
  local archive_url="https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/${archive_name}"

  rm -f /etc/apt/sources.list.d/google-cloud-sdk.list /usr/share/keyrings/cloud.google.gpg
  download_checked "$archive_url" "$archive_path" "$GOOGLE_CLOUD_CLI_ARCHIVE_SHA256"
  rm -rf /opt/google-cloud-sdk
  tar -xzf "$archive_path" -C /opt
  rm -f "$archive_path"
  /opt/google-cloud-sdk/install.sh --quiet --usage-reporting false \
    --path-update false --bash-completion false --command-completion false

  local bin
  for bin in bq docker-credential-gcloud gcloud gcloud-crc32c git-credential-gcloud.sh gsutil; do
    test -e "/opt/google-cloud-sdk/bin/$bin" || continue
    ln -sf "/opt/google-cloud-sdk/bin/$bin" "/usr/bin/$bin"
  done
  echo "google-cloud-sdk $archive_url" >>"$HELPER_SCRIPTS/apt-sources.txt"
  gcloud --version
}

install_nvm_from_archive() {
  local archive_name="nvm-v${NVM_VERSION}.tar.gz"
  local archive_path="/tmp/${archive_name}"
  local nvm_dir=/etc/skel/.nvm

  download_checked \
    "https://codeload.github.com/nvm-sh/nvm/tar.gz/refs/tags/v${NVM_VERSION}" \
    "$archive_path" \
    "$NVM_ARCHIVE_SHA256"
  install -d -m 0755 "$nvm_dir"
  tar -xzf "$archive_path" -C "$nvm_dir" --strip-components=1
  rm -f "$archive_path"

  source "$HELPER_SCRIPTS/etc-environment.sh"
  set_etc_environment_variable "NVM_DIR" '$HOME/.nvm'
  echo '[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"  # This loads nvm' >>/etc/skel/.bash_profile
  echo 'source "$NVM_DIR/nvm.sh"' >>/etc/skel/.bashrc

  export NVM_DIR="$nvm_dir"
  # shellcheck disable=SC1091
  source "$NVM_DIR/nvm.sh"
  test "$(nvm --version)" = "$NVM_VERSION"
  nvm alias default system
}

run_upstream_installer() {
  local installer_path="$1"
  local installer_name="${installer_path##*/}"
  local max_attempts=1
  case "$installer_name" in
    install-bicep.sh)
      install_bicep_from_nuget
      return
      ;;
    install-git-lfs.sh)
      install_git_lfs_from_ubuntu
      return
      ;;
    install-google-cloud-cli.sh) max_attempts=3 ;;
    install-nvm.sh)
      install_nvm_from_archive
      return
      ;;
  esac

  local attempt
  for ((attempt = 1; attempt <= max_attempts; attempt++)); do
    if bash "$installer_path"; then
      if [ "$installer_name" = install-google-cloud-cli.sh ] && ! command -v gcloud >/dev/null 2>&1; then
        echo "upstream Google Cloud installer returned success without gcloud" >&2
      else
        return 0
      fi
    fi
    if [ "$attempt" -lt "$max_attempts" ]; then
      echo "upstream installer failed; retrying $installer_name ($attempt/$max_attempts)" >&2
      sleep 10
    fi
  done
  if [ "$installer_name" = install-google-cloud-cli.sh ]; then
    echo "upstream installer failed after $max_attempts attempts; using checked Google Cloud CLI archive" >&2
    install_google_cloud_cli_from_archive
    return
  fi
  echo "upstream installer failed after $max_attempts attempts: $installer_name" >&2
  return 1
}

install_azcopy_from_microsoft_package() {
  local package=/tmp/azcopy.deb
  download_checked \
    "https://packages.microsoft.com/ubuntu/24.04/prod/pool/main/a/azcopy/azcopy_${AZCOPY_VERSION}_amd64.deb" \
    "$package" \
    "$AZCOPY_DEB_SHA256"
  install -d -m 0755 /etc/apt/preferences.d
  cat >/etc/apt/preferences.d/qiniu-azcopy <<APT_PREFERENCE
Package: azcopy
Pin: version ${AZCOPY_VERSION}
Pin-Priority: 1001
APT_PREFERENCE
  dpkg -i "$package"
  rm -f "$package"
  ln -sf "$(command -v azcopy)" /usr/local/bin/azcopy10
  test "$(azcopy --version)" = "azcopy version $AZCOPY_VERSION"
}

install_azure_devops_extension() {
  local wheel="/tmp/azure_devops-${AZURE_DEVOPS_EXTENSION_VERSION}-py2.py3-none-any.whl"
  source "$HELPER_SCRIPTS/etc-environment.sh"
  export AZURE_EXTENSION_DIR=/opt/az/azcliextensions
  set_etc_environment_variable "AZURE_EXTENSION_DIR" "$AZURE_EXTENSION_DIR"
  download_checked \
    "https://azcliprod.blob.core.windows.net/cli-extensions/azure_devops-${AZURE_DEVOPS_EXTENSION_VERSION}-py2.py3-none-any.whl" \
    "$wheel" \
    "$AZURE_DEVOPS_EXTENSION_SHA256"
  az extension add --yes --source "$wheel"
  rm -f "$wheel"
  test "$(az extension show --name azure-devops --query version -o tsv)" = "$AZURE_DEVOPS_EXTENSION_VERSION"
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

install_pester_for_upstream_tests() {
  local pester_version
  pester_version="$(
    jq -er '
      .powershellModules[]
      | select(.name == "Pester")
      | .versions[]
    ' "$INSTALLER_SCRIPT_FOLDER/toolset.json"
  )"
  PESTER_VERSION="$pester_version" pwsh -NoLogo -NoProfile -Command '
    $ErrorActionPreference = "Stop"
    Set-PSRepository -Name PSGallery -InstallationPolicy Trusted
    Install-Module -Name Pester -RequiredVersion $env:PESTER_VERSION -Scope AllUsers -SkipPublisherCheck -Force
    Import-Module Pester -RequiredVersion $env:PESTER_VERSION -Force
    if ((Get-Module Pester).Version.ToString() -ne $env:PESTER_VERSION) { exit 1 }
  '
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
  install_azcopy_from_microsoft_package
  for installer in \
    configure-apt-sources.sh \
    configure-apt.sh \
    install-apt-vital.sh \
    install-ms-repos.sh \
    configure-image-data-file.sh \
    configure-environment.sh \
    install-apt-common.sh \
    install-azure-cli.sh \
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
    run_upstream_installer "$upstream_build/$installer"
    if [ "$installer" = install-azure-cli.sh ]; then
      install_azure_devops_extension
    fi
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
run_detached_until_tcp_state() {
  desired_state="$1"
  host="$2"
  port="$3"
  shift 3
  /usr/bin/python3 - "$desired_state" "$host" "$port" "$@" <<'PYTHON'
import os
import signal
import socket
import subprocess
import sys
import time

desired_active = sys.argv[1] == "active"
host = sys.argv[2]
port = int(sys.argv[3])
process = subprocess.Popen(
    sys.argv[4:],
    stdin=subprocess.DEVNULL,
    stdout=subprocess.DEVNULL,
    stderr=subprocess.DEVNULL,
    close_fds=True,
    start_new_session=True,
)
controller_pid_file = "/tmp/qiniu-runner-build-tools/apache2-controller.pid"
with open(controller_pid_file, "w", encoding="ascii") as controller_file:
    controller_file.write(str(process.pid))

def service_is_active():
    try:
        with socket.create_connection((host, port), timeout=0.2):
            return True
    except OSError:
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
  run_detached_until_tcp_state active 127.0.0.1 80 /usr/sbin/apachectl -DFOREGROUND
}
stop_apache() {
  /usr/bin/python3 - <<'PYTHON'
import os
import signal
import socket
import sys
import time

controller_pid_file = "/tmp/qiniu-runner-build-tools/apache2-controller.pid"

def service_is_active():
    try:
        with socket.create_connection(("127.0.0.1", 80), timeout=0.2):
            return True
    except OSError:
        return False

try:
    with open(controller_pid_file, encoding="ascii") as controller_file:
        controller_pid = int(controller_file.read().strip())
except (FileNotFoundError, ValueError):
    sys.exit(1 if service_is_active() else 0)

try:
    os.killpg(controller_pid, signal.SIGTERM)
except ProcessLookupError:
    pass

deadline = time.monotonic() + 30
while time.monotonic() < deadline:
    if not service_is_active():
        try:
            os.unlink(controller_pid_file)
        except FileNotFoundError:
            pass
        sys.exit(0)
    time.sleep(0.1)
sys.exit(124)
PYTHON
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
  # The disk-bounded contract provides Ninja but excludes the full image's
  # CMake toolchain. Keep the upstream Ninja CLI assertion while skipping only
  # the two project-generation assertions that require CMake.
  ninja_test=/imagegeneration/tests/Tools.Tests.ps1
  test "$(grep -Fxc '    It "Make a simple ninja project" {' "$ninja_test" || true)" -eq 1
  test "$(grep -Fxc '    It "build.ninja file should exist" {' "$ninja_test" || true)" -eq 1
  test "$(grep -Fxc '    It "Ninja" {' "$ninja_test" || true)" -eq 1
  sed -i \
    -e 's/    It "Make a simple ninja project" {/    It "Make a simple ninja project" -Skip {/' \
    -e 's/    It "build.ninja file should exist" {/    It "build.ninja file should exist" -Skip {/' \
    "$ninja_test"
  test "$(grep -Fxc '    It "Make a simple ninja project" -Skip {' "$ninja_test" || true)" -eq 1
  test "$(grep -Fxc '    It "build.ninja file should exist" -Skip {' "$ninja_test" || true)" -eq 1

  bash "$upstream_build/install-ms-repos.sh"
  install_azcopy_from_microsoft_package
  ensure_upstream_apt_source_layout
  bash "$upstream_build/configure-apt-sources.sh"
  configure_reliable_apt_sources
  bash "$upstream_build/configure-apt.sh"
  bash "$upstream_build/configure-environment.sh"
  bash "$upstream_build/install-apt-vital.sh"
  bash "$upstream_build/install-powershell.sh"
  install_pester_for_upstream_tests
  bash "$HELPER_SCRIPTS/invoke-tests.sh" Tools azcopy

  for installer in \
    install-apt-common.sh \
    install-azure-cli.sh \
    install-bicep.sh \
    install-apache.sh \
    install-aws-tools.sh \
    install-container-tools.sh \
    install-git.sh \
    install-git-lfs.sh \
    install-github-cli.sh \
    install-google-cloud-cli.sh \
    install-nvm.sh \
    install-nodejs.sh \
    configure-dpkg.sh \
    install-yq.sh \
    install-python.sh \
    install-zstd.sh \
    install-ninja.sh; do
    run_upstream_installer "$upstream_build/$installer"
    case "$installer" in
      install-azure-cli.sh)
        install_azure_devops_extension
        bash "$HELPER_SCRIPTS/invoke-tests.sh" CLI.Tools "Azure DevOps CLI"
        ;;
      install-apache.sh) stop_validated_service apache2 ;;
    esac
  done
  . "$HELPER_SCRIPTS/etc-environment.sh"
  reload_etc_environment
  bash "$upstream_build/install-pipx-packages.sh"
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
