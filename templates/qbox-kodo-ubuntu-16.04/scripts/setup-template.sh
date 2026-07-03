#!/usr/bin/env bash
set -eux

setup_marker="/usr/local/share/qiniu-sandbox-runner-template/.setup-complete"

is_setup_complete() {
  [ -f "$setup_marker" ] || return 1
  bash -lc 'command -v git curl wget unzip ssh pkg-config nc lsof pstree redis-server cmake g++-4.9 python convert gs pdfinfo go docker dockerd ensure-docker' >/dev/null || return 1
  /usr/local/go1.12.17/bin/go version | grep -q 'go1.12.17' || return 1
  /usr/local/go1.21.13/bin/go version | grep -q 'go1.21.13' || return 1
  /usr/local/go1.23.11/bin/go version | grep -q 'go1.23.11' || return 1
  /usr/local/go1.24.6/bin/go version | grep -q 'go1.24.6' || return 1
  docker --version >/dev/null || return 1
  test -x /opt/actions-runner/config.sh || return 1
  test -x /opt/actions-runner/run.sh || return 1
}

if is_setup_complete; then
  echo "qbox kodo Ubuntu 16.04 runner template is already provisioned"
  exit 0
fi

download() {
  url="$1"
  output="$2"
  curl --http1.1 -fL --show-error --connect-timeout 15 --max-time 600 \
    --retry 5 --retry-delay 2 \
    "$url" -o "$output"
}

install_go() {
  version="$1"
  target="/usr/local/go${version}"
  if [ -x "$target/bin/go" ] && "$target/bin/go" version | grep -q "go${version}"; then
    return
  fi
  download "https://go.dev/dl/go${version}.linux-amd64.tar.gz" "/tmp/go${version}.tar.gz"
  rm -rf "$target"
  tar -C /usr/local -xzf "/tmp/go${version}.tar.gz"
  mv /usr/local/go "$target"
  rm "/tmp/go${version}.tar.gz"
  "$target/bin/go" version
}

apt-get update -y
apt-get install -y --no-install-recommends \
  software-properties-common \
  ca-certificates \
  curl
add-apt-repository -y ppa:ubuntu-toolchain-r/test
apt-get update -y
apt-get install -y --no-install-recommends \
  git \
  wget \
  unzip \
  openssh-client \
  pkg-config \
  netcat-openbsd \
  lsof \
  psmisc \
  zlib1g-dev \
  libbz2-dev \
  libsnappy-dev \
  liblz4-dev \
  redis-server \
  libfreetype6-dev \
  enca \
  g++-4.9 \
  cmake \
  libglib2.0-0 \
  ghostscript \
  libgif7 \
  python-dev \
  libevent-dev \
  imagemagick \
  poppler-utils \
  tar \
  gzip \
  xz-utils \
  sudo \
  make \
  iptables \
  docker.io

install_go "1.12.17"
install_go "1.21.13"
install_go "1.23.11"
install_go "1.24.6"
ln -sfn /usr/local/go1.24.6 /usr/local/go
ln -sfn /usr/local/go/bin/go /usr/local/bin/go
ln -sfn /usr/local/go/bin/gofmt /usr/local/bin/gofmt
cat >/etc/profile.d/go.sh <<'EOF'
export GOPATH=/opt/go
export GOBIN=/usr/local/bin
export PATH=/usr/local/go/bin:/opt/go/bin:$PATH
EOF
mkdir -p "${GOPATH}/pkg/mod" "${GOPATH}/bin"
chmod -R a+rX /usr/local/go1.12.17 /usr/local/go1.21.13 /usr/local/go1.23.11 /usr/local/go1.24.6 "${GOPATH}"

if ! id -u user >/dev/null 2>&1; then
  useradd -m -s /bin/bash user
fi
usermod -aG sudo user
usermod -aG docker user
echo "user ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/90-user
chmod 0440 /etc/sudoers.d/90-user
mkdir -p /home/user/.config/git /tmp/runner-home/.config/git /opt/hostedtoolcache /opt/actions-runner /var/lib/docker
chown -R user:user /home/user /tmp/runner-home /opt/hostedtoolcache /opt/actions-runner
install -m 0755 /usr/local/share/qiniu-sandbox-runner-template/ensure-docker /usr/local/bin/ensure-docker

runner_arch="x64"
if [ ! -x /opt/actions-runner/config.sh ] || [ ! -x /opt/actions-runner/run.sh ]; then
  download \
    "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${runner_arch}-${RUNNER_VERSION}.tar.gz" \
    /tmp/actions-runner.tar.gz
  tar xzf /tmp/actions-runner.tar.gz -C /opt/actions-runner
  rm /tmp/actions-runner.tar.gz
  /opt/actions-runner/bin/installdependencies.sh
fi
test -x /opt/actions-runner/config.sh
test -x /opt/actions-runner/run.sh
chown -R user:user /opt/actions-runner

bash -lc 'command -v git curl wget unzip ssh pkg-config nc lsof pstree redis-server cmake g++-4.9 python convert gs pdfinfo go docker dockerd ensure-docker'
/usr/local/go1.12.17/bin/go version
/usr/local/go1.21.13/bin/go version
/usr/local/go1.23.11/bin/go version
/usr/local/go1.24.6/bin/go version
go version
docker --version

apt-get clean
rm -rf /var/lib/apt/lists/*
touch "$setup_marker"
