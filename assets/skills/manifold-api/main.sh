#!/usr/bin/env bash
set -euo pipefail

# Manifold API helper script

API_URL="http:/localhost:32180"

show_help() {
  echo "Usage: $0 <command> [arguments]"
  echo ""
  echo "Commands:"
  echo "  healthz             Check API health"
  echo "  readyz              Check API readiness"
  echo "  status              Get overall API status (/api/status)"
  echo "  spec-path <path>    Get OpenAPI definition for a specific path (e.g., /api/projects)"
  echo "  spec-tags           List all tags defined in the OpenAPI spec"
}

if [ $# -lt 1 ]; then
  show_help
  exit 1
fi

COMMAND=$1

case "$COMMAND" in
  healthz)
    curl -s "${API_URL}/healthz"
    ;;
  readyz)
    curl -s "${API_URL}/readyz"
    ;;
  status)
    curl -s "${API_URL}/api/status"
    ;;
  spec-path)
    if [ $# -lt 2 ]; then
      echo "Error: spec-path requires a path argument."
      exit 1
    fi
    PATH_ARG=$2
    # Filter the OpenAPI JSON for the specific path to prevent huge outputs
    curl -s "${API_URL}/openapi.json" | jq ".paths[\"${PATH_ARG}\"]"
    ;;
  spec-tags)
    # Extract just the top-level tags and paths summary
    curl -s "${API_URL}/openapi.json" | jq '.paths | to_entries | map({path: .key, methods: (.value | keys)})'
    ;;
  help|-h|--help)
    show_help
    exit 0
    ;;
  *)
    echo "Unknown command: $COMMAND"
    show_help
    exit 1
    ;;
esac
