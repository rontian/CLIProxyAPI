#!/usr/bin/env bash
#
# prepare-env.sh - Prepares .env with git versioning information
#

set -euo pipefail

# 1. Copy .env.example to .env if .env does not exist
if [ ! -f .env ]; then
  if [ -f .env.example ]; then
    echo "Copying .env.example to .env..."
    cp .env.example .env
  else
    echo "Error: .env.example not found!"
    exit 1
  fi
fi

# 2. Get Git Versioning Information
VERSION=""
if command -v git &> /dev/null && ([ -d .git ] || git rev-parse --is-inside-work-tree &> /dev/null); then
  VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "")
fi

if [ -z "$VERSION" ]; then
  VERSION="dev"
fi

echo "Detected version: $VERSION"

# 3. Update VERSION in .env
# We use perl for portable in-place replacement (works on both macOS and Linux)
if grep -q "VERSION=" .env; then
  perl -i -pe "s/^#?\\s*VERSION=.*/VERSION=$VERSION/g" .env
  echo "Updated VERSION in .env to $VERSION"
else
  echo "" >> .env
  echo "VERSION=$VERSION" >> .env
  echo "Added VERSION=$VERSION to .env"
fi

# For CLIProxyAPI, also update COMMIT and BUILD_DATE
if [ -f config.example.yaml ]; then
  COMMIT=""
  if command -v git &> /dev/null && ([ -d .git ] || git rev-parse --is-inside-work-tree &> /dev/null); then
    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "")
  fi
  if [ -z "$COMMIT" ]; then
    COMMIT="none"
  fi
  BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

  if grep -q "COMMIT=" .env; then
    perl -i -pe "s/^#?\\s*COMMIT=.*/COMMIT=$COMMIT/g" .env
  else
    echo "COMMIT=$COMMIT" >> .env
  fi

  if grep -q "BUILD_DATE=" .env; then
    perl -i -pe "s/^#?\\s*BUILD_DATE=.*/BUILD_DATE=$BUILD_DATE/g" .env
  else
    echo "BUILD_DATE=$BUILD_DATE" >> .env
  fi
  echo "Updated COMMIT ($COMMIT) and BUILD_DATE ($BUILD_DATE) in .env"
fi

echo "Environment file preparation complete!"
