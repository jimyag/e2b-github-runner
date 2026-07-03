#!/usr/bin/env bash
set -eux

download() {
  url="$1"
  output="$2"
  curl --http1.1 -fL --show-error --connect-timeout 15 --max-time 300 \
    --retry 5 --retry-all-errors --retry-delay 2 \
    "$url" -o "$output"
}

apt-get update
apt-get install -y --no-install-recommends \
  apt-transport-https \
  ca-certificates \
  curl \
  wget \
  git \
  git-lfs \
  golang-golang-x-tools \
  gawk \
  jq \
  sudo \
  openssh-client \
  tar \
  gzip \
  unzip \
  xz-utils \
  zstd \
  rsync \
  rclone \
  coreutils \
  findutils \
  file \
  build-essential \
  pkg-config \
  make \
  cmake \
  autoconf \
  automake \
  libtool \
  lsb-release \
  gnupg \
  iptables \
  iproute2 \
  docker.io \
  docker-buildx \
  docker-compose-v2 \
  python3 \
  python3-pip \
  python3-venv \
  python3-dev

install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
  | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg
echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_MAJOR}.x nodistro main" \
  > /etc/apt/sources.list.d/nodesource.list
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
  | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
  > /etc/apt/sources.list.d/github-cli.list
apt-get update
apt-get install -y --no-install-recommends nodejs gh
git lfs install --system

download \
  "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
  /tmp/go.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf /tmp/go.tar.gz
rm /tmp/go.tar.gz
mkdir -p "${GOPATH}/pkg/mod" "${GOPATH}/bin"
go version
download \
  "https://github.com/go-task/task/releases/download/v${TASK_VERSION}/task_linux_amd64.tar.gz" \
  /tmp/task.tar.gz
tar -C /usr/local/bin -xzf /tmp/task.tar.gz task
chmod 0755 /usr/local/bin/task
rm /tmp/task.tar.gz
download \
  "https://github.com/mvdan/gofumpt/releases/download/v${GOFUMPT_VERSION}/gofumpt_v${GOFUMPT_VERSION}_linux_amd64" \
  /tmp/gofumpt
install -m 0755 /tmp/gofumpt /usr/local/bin/gofumpt
rm /tmp/gofumpt
download \
  "https://github.com/dominikh/go-tools/releases/download/${STATICCHECK_RELEASE}/staticcheck_linux_amd64.tar.gz" \
  /tmp/staticcheck.tar.gz
tar -C /tmp -xzf /tmp/staticcheck.tar.gz staticcheck/staticcheck
install -m 0755 /tmp/staticcheck/staticcheck /usr/local/bin/staticcheck
rm -rf /tmp/staticcheck /tmp/staticcheck.tar.gz
chmod -R a+rX "${GOPATH}"

mkdir -p "${OPENTOFU_CACHE_DIR}/${OPENTOFU_VERSION}" "${TERRAFORM_CACHE_DIR}/${TERRAFORM_VERSION}"
download \
  "https://github.com/opentofu/opentofu/releases/download/v${OPENTOFU_VERSION}/tofu_${OPENTOFU_VERSION}_linux_amd64.zip" \
  /tmp/tofu.zip
unzip -qo /tmp/tofu.zip -d "${OPENTOFU_CACHE_DIR}/${OPENTOFU_VERSION}"
install -m 0755 "${OPENTOFU_CACHE_DIR}/${OPENTOFU_VERSION}/tofu" /usr/local/bin/tofu
download \
  "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip" \
  /tmp/terraform.zip
unzip -qo /tmp/terraform.zip -d "${TERRAFORM_CACHE_DIR}/${TERRAFORM_VERSION}"
install -m 0755 "${TERRAFORM_CACHE_DIR}/${TERRAFORM_VERSION}/terraform" /usr/local/bin/terraform
rm -f /tmp/tofu.zip /tmp/terraform.zip

install -m 0755 /usr/local/share/qiniu-sandbox-runner-template/ensure-docker /usr/local/bin/ensure-docker

if ! id -u user >/dev/null 2>&1; then
  useradd -m -s /bin/bash user
fi
usermod -aG sudo user
usermod -aG docker user
echo "user ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/90-user
chmod 0440 /etc/sudoers.d/90-user
mkdir -p /home/user/.config/git /tmp/runner-home/.config/git /opt/hostedtoolcache /opt/actions-runner /var/lib/docker
chown -R user:user /home/user /tmp/runner-home /opt/hostedtoolcache /opt/actions-runner

runner_arch="x64"
download \
  "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${runner_arch}-${RUNNER_VERSION}.tar.gz" \
  /tmp/actions-runner.tar.gz
tar xzf /tmp/actions-runner.tar.gz -C /opt/actions-runner
rm /tmp/actions-runner.tar.gz
/opt/actions-runner/bin/installdependencies.sh
test -x /opt/actions-runner/config.sh
test -x /opt/actions-runner/run.sh
chown -R user:user /opt/actions-runner

bash -lc 'command -v go task gofumpt goimports staticcheck node npm gh jq docker rclone tofu terraform'
go version
task --version
gofumpt --version
staticcheck -version
node --version
npm --version
gh --version
docker --version
rclone version
tofu version
terraform version

apt-get clean
rm -rf /var/lib/apt/lists/*
