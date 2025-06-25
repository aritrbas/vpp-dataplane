#!/bin/bash

# Script to cleanup all Kind clusters
# Usage: ./cleanup_clusters.sh

set -e  # Exit on any error

echo "This script cleans up all Kind clusters"

# Step 1: Get list of existing clusters
echo "Step 1: Checking for existing Kind clusters..."
clusters_output=$(kind get clusters 2>&1 || echo "")

# Check if no clusters exist
if echo "$clusters_output" | grep -q "No kind clusters found."; then
    echo "✓ No running clusters found. Nothing to clean up."
    exit 0
fi

# Extract cluster names (each line is a cluster name)
cluster_names=$(echo "$clusters_output" | grep -v "^$" | tr '\n' ' ')

if [ -z "$cluster_names" ]; then
    echo "✓ No running clusters found. Nothing to clean up."
    exit 0
fi

echo "Found clusters: $cluster_names"

# Step 2: Delete each cluster
echo "Step 2: Deleting all Kind clusters..."
for cluster in $cluster_names; do
    if [ -n "$cluster" ]; then
        kind delete cluster --name="$cluster"
        echo "✓ Successfully deleted cluster '$cluster'"
    fi
done

# Step 3: Verify cleanup
echo "Step 3: Verifying cleanup..."
sleep 2

final_check=$(kind get clusters 2>&1 || echo "")
if echo "$final_check" | grep -q "No kind clusters found."; then
    echo "✓ Successfully cleaned up all clusters. No kind clusters found."
else
    echo "⚠ Warning: Some clusters may still exist:"
    echo "$final_check"
    exit 1
fi

echo "✓ Cleanup completed successfully!"
