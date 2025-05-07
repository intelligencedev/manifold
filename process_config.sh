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

# Extract all paths from the YAML file with their original casing
echo "Building YAML path dictionary..."
declare -A path_map
while IFS= read -r path; do
    if [ ! -z "$path" ]; then
        # Store both formats - with dots and with underscores
        path_lower=$(echo "$path" | tr '[:upper:]' '[:lower:]')
        path_map["$path_lower"]="$path"
    fi
done < <(yq eval '.. | path | select(length > 0) | join(".")' "$CONFIG_TEMPLATE")

# Debug: Print all paths found
echo "Found paths in YAML:"
for path in "${!path_map[@]}"; do
    echo "  $path -> ${path_map[$path]}"
done

# Process environment variables prefixed with MANIFOLD__
for var in $(env | grep ^MANIFOLD__ | cut -d= -f1); do
    # Remove MANIFOLD__ prefix
    key_without_prefix=$(echo "$var" | sed 's/^MANIFOLD__//')
    
    # Convert double underscore to dots for nested paths
    env_path=$(echo "$key_without_prefix" | tr '[:upper:]' '[:lower:]' | sed 's/__/./g')
    
    # Check if this exact path exists in our dictionary
    yaml_path=""
    if [ -n "${path_map[$env_path]}" ]; then
        # Direct match found
        yaml_path="${path_map[$env_path]}"
        echo "Direct match found: $env_path -> $yaml_path"
    else
        # No direct match, try fuzzy matching
        for path_key in "${!path_map[@]}"; do
            # Compare normalized versions (all lowercase, no underscores vs dots)
            path_norm=$(echo "$path_key" | sed 's/\./_/g')
            env_norm=$(echo "$env_path" | sed 's/\./_/g')
            
            if [ "$path_norm" = "$env_norm" ]; then
                yaml_path="${path_map[$path_key]}"
                echo "Fuzzy match found: $env_path -> $yaml_path (normalized: $path_norm)"
                break
            fi
        done
    fi
    
    # If no match found, use the normalized path
    if [ -z "$yaml_path" ]; then
        yaml_path="$env_path"
        echo "Warning: No match found for $var, using $yaml_path"
    fi
    
    # Get environment variable value
    value="${!var}"
    echo "Setting $yaml_path = $value"
    
    # Special handling for arrays/objects
    if [[ "$value" == \[* ]] || [[ "$value" == \{* ]]; then
        # Handle as JSON - assumed to be valid JSON
        yq -i ".$yaml_path = $value" "$CONFIG_OUTPUT"
    else
        # Handle as scalar value
        # Properly quote strings if needed
        if [[ "$value" =~ ^[0-9]+$ ]] || [[ "$value" == "true" ]] || [[ "$value" == "false" ]] || [[ "$value" == "null" ]]; then
            # Numeric or boolean values don't need quotes
            yq -i ".$yaml_path = $value" "$CONFIG_OUTPUT"
        else
            # String values need quotes
            yq -i ".$yaml_path = \"$value\"" "$CONFIG_OUTPUT"
        fi
    fi
done

echo "Config file processed successfully!"