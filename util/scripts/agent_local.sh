#!/bin/bash

# Script to build and push Calico VPP agent Docker image to local registry
# Usage: ./agent_local.sh <dockerID> <tagname>

set -e  # Exit on any error

echo "This script builds the Calico VPP agent Docker image and pushes it to a local Docker registry"
# Check if required arguments are provided
if [ $# -ne 2 ]; then
    echo "Usage: $0 <dockerID> <tagname>"
    echo "Example: $0 aritra21295 abasu-test99"
    exit 1
fi

DOCKER_ID="$1"
TAG_NAME="$2"
LOCAL_REGISTRY="localhost:5000"

echo "Building and pushing Calico VPP agent with Docker ID: $DOCKER_ID, tag: $TAG_NAME to local registry: $LOCAL_REGISTRY"

# Step 1: Navigate to home directory
echo "Step 1: Navigating to home directory..."
cd ~

# Step 2: Export Go binary path
echo "Step 2: Setting up Go path..."
export PATH=$PATH:/usr/local/go/bin

# Step 3: Navigate to calico-vpp-agent directory
echo "Step 3: Navigating to calico-vpp-agent directory..."
cd ~/vpp-dataplane/calico-vpp-agent

# Step 4: Build the image
echo "Step 4: Building the image with tag $TAG_NAME..."
make TAG="$TAG_NAME" image

# Wait for the specific output line
echo "Waiting for build completion..."
# Wait for the make command to complete and verify the naming output
while ! docker images | grep -q "calicovpp/agent.*$TAG_NAME"; do
        sleep 1
done
echo "✓ Successfully created image docker.io/calicovpp/agent:$TAG_NAME"

# Step 5: Tag the image for local registry
echo "Step 5: Tagging image for local registry $LOCAL_REGISTRY..."
docker tag "docker.io/calicovpp/agent:$TAG_NAME" "$LOCAL_REGISTRY/$DOCKER_ID/agent:$TAG_NAME"

# Step 6: Push the image to local registry
echo "Step 6: Pushing image to local registry..."
docker push "$LOCAL_REGISTRY/$DOCKER_ID/agent:$TAG_NAME"

echo "✓ Successfully built and pushed $LOCAL_REGISTRY/$DOCKER_ID/agent:$TAG_NAME"
