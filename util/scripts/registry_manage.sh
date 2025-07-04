#!/bin/bash

# Registry Management Script
# Manages a local Docker registry running on port 5000

REGISTRY_URL="localhost:5000"
REGISTRY_API_URL="http://${REGISTRY_URL}/v2"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to check if registry is running
check_registry() {
    if ! docker ps --filter "name=local-registry" --format "{{.Names}}" | grep -q "^local-registry$"; then
        echo -e "${RED}✗ Error: Local docker registry is not running${NC}"
        echo "Please start the registry first using registry_start.sh"
        exit 1
    fi

    # Check if registry API is accessible
    if ! curl -s "${REGISTRY_API_URL}/" > /dev/null 2>&1; then
        echo -e "${RED}✗ Error: Registry API is not accessible at ${REGISTRY_API_URL}${NC}"
        exit 1
    fi
}

# Function to list all repositories in the registry
list_repositories() {
    echo -e "${BLUE}📋 Listing all repositories in the registry...${NC}"

    repositories=$(curl -s "${REGISTRY_API_URL}/_catalog" | jq -r '.repositories[]?' 2>/dev/null)

    if [ -z "$repositories" ]; then
        echo -e "${YELLOW}⚠ No repositories found in the registry${NC}"
        return
    fi

    echo -e "${GREEN}Found repositories:${NC}"
    echo "$repositories" | while read -r repo; do
        echo "  - $repo"

        # Get tags for each repository
        tags=$(curl -s "${REGISTRY_API_URL}/${repo}/tags/list" | jq -r '.tags[]?' 2>/dev/null)
        if [ -n "$tags" ]; then
            echo "$tags" | while read -r tag; do
                echo "    └── $tag"
            done
        else
            echo "    └── (no tags)"
        fi
    done
}

# Function to list detailed information about repositories
list_detailed() {
    echo -e "${BLUE}📋 Listing detailed repository information...${NC}"

    repositories=$(curl -s "${REGISTRY_API_URL}/_catalog" | jq -r '.repositories[]?' 2>/dev/null)

    if [ -z "$repositories" ]; then
        echo -e "${YELLOW}⚠ No repositories found in the registry${NC}"
        return
    fi

    echo "$repositories" | while read -r repo; do
        echo -e "${GREEN}Repository: $repo${NC}"

        tags=$(curl -s "${REGISTRY_API_URL}/${repo}/tags/list" | jq -r '.tags[]?' 2>/dev/null)
        if [ -n "$tags" ]; then
            echo "$tags" | while read -r tag; do
                # Get manifest digest
                digest=$(curl -s -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
                    "${REGISTRY_API_URL}/${repo}/manifests/${tag}" | \
                    jq -r '.config.digest' 2>/dev/null)

                echo "  Tag: $tag"
                echo "    Digest: ${digest:-"unknown"}"
                echo "    Full name: ${REGISTRY_URL}/${repo}:${tag}"
            done
        else
            echo "  (no tags)"
        fi
        echo
    done
}

# Function to remove a specific image/tag
remove_image() {
    local repo_tag="$1"

    if [ -z "$repo_tag" ]; then
        echo -e "${RED}✗ Error: Please specify repository:tag to remove${NC}"
        echo "Usage: $0 remove <repository:tag>"
        exit 1
    fi

    # Split repo:tag
    local repo="${repo_tag%:*}"
    local tag="${repo_tag##*:}"

    if [ "$repo" = "$tag" ]; then
        echo -e "${RED}✗ Error: Please specify both repository and tag (format: repository:tag)${NC}"
        exit 1
    fi

    echo -e "${YELLOW}🗑️  Removing ${repo}:${tag}...${NC}"

    # Get the manifest digest
    local digest=$(curl -s -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
        -I "${REGISTRY_API_URL}/${repo}/manifests/${tag}" | \
        grep -i "docker-content-digest" | cut -d' ' -f2 | tr -d '\r')

    if [ -z "$digest" ]; then
        echo -e "${RED}✗ Error: Could not find manifest for ${repo}:${tag}${NC}"
        exit 1
    fi

    # Delete the manifest
    local response=$(curl -s -w "%{http_code}" -X DELETE "${REGISTRY_API_URL}/${repo}/manifests/${digest}")
    local http_code="${response: -3}"

    if [ "$http_code" = "202" ]; then
        echo -e "${GREEN}✓ Successfully removed ${repo}:${tag}${NC}"
    else
        echo -e "${RED}✗ Error: Failed to remove ${repo}:${tag} (HTTP ${http_code})${NC}"
        exit 1
    fi
}

# Function to clear entire registry
clear_registry() {
    echo -e "${YELLOW}⚠️  WARNING: This will remove ALL images from the registry!${NC}"
    read -p "Are you sure you want to continue? (yes/no): " confirm

    if [ "$confirm" != "yes" ]; then
        echo "Operation cancelled."
        exit 0
    fi

    echo -e "${YELLOW}🗑️  Clearing entire registry...${NC}"

    # Stop the registry container
    echo "Stopping registry container..."
    docker stop local-registry > /dev/null 2>&1

    # Remove the registry data directory
    echo "Removing registry data..."
    sudo rm -rf /tmp/docker_registry/*

    # Restart the registry container
    echo "Restarting registry container..."
    docker start local-registry > /dev/null 2>&1

    # Wait for registry to be ready
    echo "Waiting for registry to restart..."
    timeout=10
    elapsed=0

    while [ $elapsed -lt $timeout ]; do
        if curl -s "${REGISTRY_API_URL}/" > /dev/null 2>&1; then
            echo -e "${GREEN}✓ Registry cleared and restarted successfully${NC}"
            return
        fi

        sleep 1
        elapsed=$((elapsed + 1))
    done

    echo -e "${RED}✗ Error: Registry failed to restart properly${NC}"
    exit 1
}



# Function to show usage
show_usage() {
    echo "Local Docker Registry Management Script"
    echo
    echo "Usage: $0 <command> [options]"
    echo
    echo "Commands:"
    echo "  list                   List all repositories and tags"
    echo "  list-detailed          List repositories with detailed information"
    echo "  remove <repo:tag>      Remove a specific image"
    echo "  clear                  Clear entire registry (removes all images)"
    echo "  help                   Show this help message"
    echo
    echo "Examples:"
    echo "  $0 list"
    echo "  $0 remove myapp:latest"
    echo "  $0 clear"
}

# Main script logic
case "${1:-help}" in
    "list")
        check_registry
        list_repositories
        ;;
    "list-detailed")
        check_registry
        list_detailed
        ;;
    "remove")
        check_registry
        remove_image "$2"
        ;;
    "clear")
        check_registry
        clear_registry
        ;;
    "help"|"--help"|"-h")
        show_usage
        ;;
    *)
        echo -e "${RED}✗ Error: Unknown command '$1'${NC}"
        echo
        show_usage
        exit 1
        ;;
esac
