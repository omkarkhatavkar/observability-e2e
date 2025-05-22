#!/bin/bash

# Define default values for RKE2 and Cert Manager versions
DEFAULT_RKE2_VERSION="v1.32.2+rke2r1"
DEFAULT_CERT_MANAGER_VERSION="v1.15.3"

# Allow overriding the default versions via environment variables or script arguments
RKE2_VERSION="${1:-$DEFAULT_RKE2_VERSION}"
CERT_MANAGER_VERSION="${2:-$DEFAULT_CERT_MANAGER_VERSION}"

echo "🚀 Installing RKE2 version: $RKE2_VERSION"
echo "🔐 Installing Cert Manager version: $CERT_MANAGER_VERSION"

# Install RKE2
curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION=$RKE2_VERSION sh -
systemctl enable --now rke2-server.service
systemctl restart rke2-server

# Configure kubectl and kubeconfig
mkdir -p ~/.kube
ln -s /etc/rancher/rke2/rke2.yaml ~/.kube/config
ln -s /var/lib/rancher/rke2/bin/kubectl /usr/local/bin/

# Install Helm
echo "📦 Installing Helm..."
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh
rm -f get_helm.sh

# Add Rancher Helm repositories
# https://ranchermanager.docs.rancher.com/getting-started/installation-and-upgrade/other-installation-methods/air-gapped-helm-cli-install/install-rancher-ha#1-add-the-helm-chart-repository

echo "📌 Adding Rancher Helm repositories..."
helm repo add rancher-latest https://releases.rancher.com/server-charts/latest
helm repo add rancher-stable https://releases.rancher.com/server-charts/stable
helm repo add rancher-latest-alpha https://releases.rancher.com/server-charts/alpha

helm repo update

# Install Cert Manager
echo "🔧 Installing Cert Manager version: $CERT_MANAGER_VERSION"
kubectl apply -f "https://github.com/cert-manager/cert-manager/releases/download/$CERT_MANAGER_VERSION/cert-manager.yaml"

# Create Rancher namespace
kubectl create namespace cattle-system

echo "✅ Installation complete! RKE2 and Rancher are now set up."
