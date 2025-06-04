#!/bin/bash

set -x
echo "============================================="
echo "Running as user: $(whoami)"
echo "============================================="

sudo apt-get update -qq && sudo apt-get install -y -qq jq

# Positional args from Terraform
RKE2_VERSION="${1}"
CERT_MANAGER_VERSION="${2}"
RANCHER_VERSION="${3}"
RANCHER_PASSWORD="${4}"
HELM_REPO_URL="${5}"

REPO_NAME="rancher"

echo "🚀 Installing RKE2 version: $RKE2_VERSION"
echo "🔐 Installing Cert Manager version: $CERT_MANAGER_VERSION"
echo "📦 Using Helm repo URL: $HELM_REPO_URL"
echo "📦 Installing Rancher version: $RANCHER_VERSION"

# Install RKE2
curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION=$RKE2_VERSION sh -
systemctl enable --now rke2-server.service
systemctl restart rke2-server

# Configure kubectl
mkdir -p ~/.kube
ln -sf /etc/rancher/rke2/rke2.yaml ~/.kube/config
ln -sf /var/lib/rancher/rke2/bin/kubectl /usr/local/bin/

# Install Helm
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh
rm -f get_helm.sh

# Add Helm repos
helm repo add "$REPO_NAME" "$HELM_REPO_URL"
helm repo add jetstack https://charts.jetstack.io
helm repo update

# Install Cert Manager
kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/$CERT_MANAGER_VERSION/cert-manager.yaml"
sleep 60 # Wait for cert-manager components to initialize

# Create Rancher namespace
kubectl create namespace cattle-system || true

PUBLIC_IP=$(curl -s ifconfig.me)
RANCHER_HOSTNAME="rancher.${PUBLIC_IP}.sslip.io"

# Determine Helm install command based on repo source
if echo "$HELM_REPO_URL" | grep -q "releases.rancher.com"; then
  echo "📦 Installing Rancher using official release chart..."
  helm install rancher rancher/rancher --namespace cattle-system \
    --version "$(echo "$RANCHER_VERSION" | tr -d 'v')" \
    --set hostname=$RANCHER_HOSTNAME \
    --set replicas=2 \
    --set bootstrapPassword=$RANCHER_PASSWORD \
    --set global.cattle.psp.enabled=false \
    --set insecure=true \
    --set rancherImage='rancher/rancher' \
    --wait \
    --timeout=10m \
    --create-namespace \
    --devel
else
  echo "📦 Installing Rancher using SUSE private registry chart..."
  helm install rancher rancher/rancher --namespace cattle-system \
    --version "$(echo "$RANCHER_VERSION" | tr -d 'v')" \
    --set hostname=$RANCHER_HOSTNAME \
    --set replicas=2 \
    --set bootstrapPassword="$RANCHER_PASSWORD" \
    --set global.cattle.psp.enabled=false \
    --set insecure=true \
    --set rancherImageTag="$RANCHER_VERSION" \
    --set rancherImage='stgregistry.suse.com/rancher/rancher' \
    --set rancherImagePullPolicy=Always \
    --set extraEnv[0].name=CATTLE_AGENT_IMAGE \
    --set extraEnv[0].value="stgregistry.suse.com/rancher/rancher-agent:$RANCHER_VERSION" \
    --wait \
    --timeout=10m \
    --create-namespace \
    --devel
fi

sleep 180
echo "✅ Rancher installation complete."

set -euo pipefail

# Required inputs
RANCHER_HOSTNAME="rancher.${PUBLIC_IP}.sslip.io"
RANCHER_URL="https://${RANCHER_HOSTNAME}"

echo "::add-mask::$RANCHER_PASSWORD"

# 1. Login with admin credentials
LOGIN_RESPONSE=$(curl --silent -X POST -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"${RANCHER_PASSWORD}\"}" \
  "${RANCHER_URL}/v3-public/localProviders/local?action=login" \
  --insecure)

TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r .token)

if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo "❌ Failed to login with admin password" >&2
  exit 1
fi

echo "::add-mask::$TOKEN"

# 2. Accept telemetry EULA
curl --silent -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"telemetry-opt","value":"out"}' \
  "${RANCHER_URL}/v3/settings/telemetry-opt" --insecure

# 3. Explicitly mark first login as complete (optional)
curl --silent -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"value":"false"}' \
  "${RANCHER_URL}/v3/settings/first-login" --insecure

# 4. Set Rancher Server URL
curl --silent -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"server-url\",\"value\":\"${RANCHER_URL}\"}" \
  "${RANCHER_URL}/v3/settings/server-url" --insecure
