#!/bin/bash
set -e

CONFIG_TEMPLATE="./config.yaml.example"
CONFIG_OUTPUT="./config.yaml"

# Check if yq is installed
if ! command -v yq &> /dev/null; then
    echo "yq could not be found, but is required to run this script."
    exit 1
fi

# Copy the template to start with
cp "$CONFIG_TEMPLATE" "$CONFIG_OUTPUT"

# Process environment variables prefixed with MANIFOLD_
for var in $(env | grep ^MANIFOLD_ | cut -d= -f1); do
    # Convert environment variable name to YAML path
    # Remove MANIFOLD_ prefix and convert to lowercase
    yaml_path=$(echo "$var" | sed 's/^MANIFOLD_//' | tr '[:upper:]' '[:lower:]' | sed 's/_/./g')
    
    # Get environment variable value
    value="${!var}"
    
    # Special handling for arrays/objects
    if [[ "$value" == \[* ]] || [[ "$value" == \{* ]]; then
        # Handle as JSON - assumed to be valid JSON
        echo "$yaml_path: $value" | yq -i eval-all 'select(fileIndex == 0) * select(fileIndex == 1)' "$CONFIG_OUTPUT" -
    else
        # Handle as scalar value
        # Properly quote strings if needed
        if [[ "$value" =~ ^[0-9]+$ ]] || [[ "$value" == "true" ]] || [[ "$value" == "false" ]] || [[ "$value" == "null" ]]; then
            # Numeric or boolean values don't need quotes
            echo "$yaml_path: $value" | yq -i eval-all 'select(fileIndex == 0) * select(fileIndex == 1)' "$CONFIG_OUTPUT" -
        else
            # String values need quotes
            echo "$yaml_path: '$value'" | yq -i eval-all 'select(fileIndex == 0) * select(fileIndex == 1)' "$CONFIG_OUTPUT" -
        fi
    fi
done

echo "Config file processed successfully!"
cat "$CONFIG_OUTPUT"