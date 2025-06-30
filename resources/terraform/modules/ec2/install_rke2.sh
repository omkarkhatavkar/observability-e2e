#!/bin/bash

set -euo pipefail

RKE2_VERSION="${1}"
CERT_MANAGER_VERSION="${2}"
HELM_REPO_URL="${3}"

echo "🚀 Installing RKE2 version: $RKE2_VERSION"
echo "🔐 Installing Cert Manager version: $CERT_MANAGER_VERSION"

sudo add-apt-repository -y universe
sudo apt-get update -qq && sudo apt-get install -y -qq jq curl
sudo wget -qO /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64 && sudo chmod +x /usr/local/bin/yq


# Install RKE2
export INSTALL_RKE2_VERSION="$RKE2_VERSION"
curl -sfL https://get.rke2.io | sh -
sudo systemctl enable --now rke2-server.service
sudo systemctl restart rke2-server

# Wait a bit to ensure RKE2 starts up and generates kubeconfig
sleep 10

# Give permissions so Terraform can copy it
cp /etc/rancher/rke2/rke2.yaml /tmp/
sudo chown ubuntu:ubuntu /tmp/rke2.yaml

### 🔧 Patch kubeconfig with external IP
EXTERNAL_IP=$(curl -s ifconfig.me)
sudo sed -i "s/127.0.0.1/${EXTERNAL_IP}/" /tmp/rke2.yaml
yq e '.clusters[].cluster |= {"server": .server, "insecure-skip-tls-verify": true}' -i /tmp/rke2.yaml

# Configure kubectl for current user (ubuntu)
mkdir -p ~/.kube
ln -sf /etc/rancher/rke2/rke2.yaml ~/.kube/config
ln -sf /var/lib/rancher/rke2/bin/kubectl /usr/local/bin/kubectl

# Install Helm
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod +x get_helm.sh
./get_helm.sh
rm -f get_helm.sh

# Add cert-manager repo and install
helm repo add jetstack https://charts.jetstack.io
helm repo update

# Add Helm repo for Rancher
helm repo add rancher "$HELM_REPO_URL"
helm repo update

# Install cert-manager
kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/$CERT_MANAGER_VERSION/cert-manager.yaml"

echo "✅ RKE2 and Cert Manager installed. Wait ~60 seconds before installing Rancher."
sleep 60
