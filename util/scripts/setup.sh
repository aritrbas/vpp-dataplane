#!/bin/bash

# Script to set up Kind cluster with Calico VPP
# Usage: ./setup_cluster.sh

set -e  # Exit on any error

echo "This script sets up a Kind cluster with Calico VPP components"

# Step 1: Create Kind cluster
echo "Step 1: Creating Kind cluster 'abasu-test'..."
kind create cluster --name=abasu-test --config=/home/aritrbas/calico/config.yaml

echo "Waiting for cluster to be ready..."
sleep 5

# Step 2: Apply Tigera operator
echo "Step 2: Applying Tigera operator..."
kubectl apply -f /home/aritrbas/calico/tigera_operator.yaml

echo "Waiting 20 seconds for Tigera operator to initialize..."
sleep 20

# Step 3: Apply Calico installation
echo "Step 3: Applying Calico installation..."
kubectl apply -f /home/aritrbas/calico/installation-default.yaml

echo "Waiting 20 seconds for Calico installation to complete..."
sleep 20

# Verify cluster creation by checking nodes
echo "Verifying cluster is Ready..."
max_attempts=10
attempt=0
while [ $attempt -lt $max_attempts ]; do
    total_nodes=$(kubectl get nodes --no-headers 2>/dev/null | wc -l || echo "0")
    nodes_ready=$(kubectl get nodes --no-headers 2>/dev/null | awk '$2 == "Ready" {count++} END {print count+0}' || echo "0")

    if [ "$nodes_ready" -eq "$total_nodes" ]; then
        echo "✓ Successfully verified all $total_nodes nodes are in Ready state"
        break
    fi

    echo "Waiting for all nodes to be ready... (attempt $((attempt + 1))/$max_attempts)"
    sleep 2
    attempt=$((attempt + 1))
done

if [ $attempt -eq $max_attempts ]; then
    echo "✗ Failed to verify all required nodes after $max_attempts attempts"
    echo "Current nodes:"
    kubectl get nodes || echo "Failed to get nodes"
    exit 1
fi

# Step 4: Apply Calico VPP
echo "Step 4: Applying Calico VPP configuration..."
kubectl apply -f /home/aritrbas/calico/calico-vpp-kind.yaml

echo "Verifying all pods are Running..."
max_attempts=10
attempt=0
while [ $attempt -lt $max_attempts ]; do
    total_pods=$(kubectl get pods -A --no-headers 2>/dev/null | wc -l || echo "0")
    pods_running=$(kubectl get pods -A --no-headers 2>/dev/null | awk '$4 == "Running" {count++} END {print count+0}' || echo "0")

    if [ "$pods_running" -eq "$total_pods" ]; then
        echo "✓ Successfully verified all $total_pods pods are in Running state"
        break
    fi

    echo "Waiting for all pods to be Running... (attempt $((attempt + 1))/$max_attempts)"
    sleep 30
    attempt=$((attempt + 1))
done

if [ $attempt -eq $max_attempts ]; then
    echo "✗ Failed to verify all required pods after $max_attempts attempts"
    echo "Current pods:"
    kubectl get pods -A || echo "Failed to get pods"
    exit 1
fi

echo "✓ Successfully completed cluster setup with Calico VPP"
echo "✓ Cluster 'abasu-test' is ready!"

# Step 5: Show cluster status
echo ""
echo "Cluster status:"
kubectl get nodes -o wide
kubectl get pods -A -o wide
