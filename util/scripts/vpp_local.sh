#!/bin/bash

# Script to build and push Calico VPP manager Docker image to local registry
# Usage: ./vpp_local.sh <dockerID> <tagname>

set -e  # Exit on any error

echo "This script builds the Calico VPP manager Docker image and pushes it to a local Docker registry"
# Check if required arguments are provided
if [ $# -ne 2 ]; then
    echo "Usage: $0 <dockerID> <tagname>"
    echo "Example: $0 aritra21295 abasu-test99"
    exit 1
fi

DOCKER_ID="$1"
TAG_NAME="$2"
LOCAL_REGISTRY="localhost:5000"

echo "Building and pushing Calico VPP manager with Docker ID: $DOCKER_ID, tag: $TAG_NAME to local registry: $LOCAL_REGISTRY"

# Step 1: Navigate to home directory
echo "Step 1: Navigating to home directory..."
cd ~

# Step 2: Export Go binary path
echo "Step 2: Setting up Go path..."
export PATH=$PATH:/usr/local/go/bin

# Step 3: Navigate to vpp-manager directory
echo "Step 3: Navigating to vpp-manager directory..."
cd ~/vpp-dataplane/vpp-manager

# Step 4: Remove existing vpp_build directory
echo "Step 4: Removing existing vpp_build directory..."
rm -rf ~/vpp-dataplane/vpp-manager/vpp_build

# Step 5: Build the image
echo "Step 5: Building the image with tag $TAG_NAME..."
make TAG="$TAG_NAME" image

# Wait for the specific output line
echo "Waiting for build completion..."
# Wait for the make command to complete and verify the naming output
while ! docker images | grep -q "calicovpp/vpp.*$TAG_NAME"; do
        sleep 1
done
echo "✓ Successfully created image docker.io/calicovpp/vpp:$TAG_NAME"

# Step 6: Tag the image for local registry
echo "Step 6: Tagging image for local registry $LOCAL_REGISTRY..."
docker tag "docker.io/calicovpp/vpp:$TAG_NAME" "$LOCAL_REGISTRY/$DOCKER_ID/vpp:$TAG_NAME"

# Step 7: Push the image to local registry
echo "Step 7: Pushing image to local registry..."
docker push "$LOCAL_REGISTRY/$DOCKER_ID/vpp:$TAG_NAME"

echo "✓ Successfully built and pushed $LOCAL_REGISTRY/$DOCKER_ID/vpp:$TAG_NAME"
