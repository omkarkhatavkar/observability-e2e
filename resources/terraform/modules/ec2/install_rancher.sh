#!/bin/bash

set -euo pipefail

# Positional args

RANCHER_VERSION="${1}"
RANCHER_PASSWORD="${2}"
HELM_REPO_URL="${3}"
INSTALL_RANCHER="${4:-true}"  # Default to true if not provided

if [[ "$INSTALL_RANCHER" != "true" ]]; then
  echo "⏭️ Skipping Rancher installation because INSTALL_RANCHER=$INSTALL_RANCHER"
  exit 0
fi

# Proceed with Rancher installation
echo "📦 Installing Rancher version: $RANCHER_VERSION"
echo "🔐 Using password: [REDACTED]"
echo "📦 Helm repo: $HELM_REPO_URL"

# Add Helm repo for Rancher
helm repo add rancher "$HELM_REPO_URL"
helm repo update

kubectl create namespace cattle-system || true

# Get public IP and set hostname
PUBLIC_IP=$(curl -s ifconfig.me)
RANCHER_HOSTNAME="rancher.${PUBLIC_IP}.sslip.io"

# Install Rancher
if echo "$HELM_REPO_URL" | grep -q "releases.rancher.com"; then
  echo "📦 Installing Rancher using official release chart..."
  helm install rancher rancher/rancher --namespace cattle-system \
    --version "$(echo "$RANCHER_VERSION" | tr -d 'v')" \
    --set hostname=$RANCHER_HOSTNAME \
    --set replicas=2 \
    --set bootstrapPassword=$RANCHER_PASSWORD \
    --set global.cattle.psp.enabled=false \
    --set insecure=true \
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

# Wait for Rancher to start
sleep 180

# Post-install setup
RANCHER_URL="https://${RANCHER_HOSTNAME}"
echo "::add-mask::$RANCHER_PASSWORD"

LOGIN_RESPONSE=$(curl --silent -X POST -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"${RANCHER_PASSWORD}\"}" \
  "${RANCHER_URL}/v3-public/localProviders/local?action=login" \
  --insecure)

TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r .token)
echo "::add-mask::$TOKEN"

if [[ -z "$TOKEN" || "$TOKEN" == "null" ]]; then
  echo "❌ Failed to login with admin password" >&2
  exit 1
fi

# Accept telemetry
curl --silent -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"telemetry-opt","value":"out"}' \
  "${RANCHER_URL}/v3/settings/telemetry-opt" --insecure

# Mark first login complete
curl --silent -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"value":"false"}' \
  "${RANCHER_URL}/v3/settings/first-login" --insecure

# Set Rancher server URL
curl --silent -X PUT -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"server-url\",\"value\":\"${RANCHER_URL}\"}" \
  "${RANCHER_URL}/v3/settings/server-url" --insecure

echo "✅ Rancher installation and configuration complete."
