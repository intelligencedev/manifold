#!/bin/sh
# Vault Initialization and Unseal Script
# This script handles first-time initialization and subsequent unsealing.
#
# WARNING: In production, use proper secret management for unseal keys!
# Options include: Vault auto-unseal with cloud KMS, HSM, or manual unsealing.

set -e

VAULT_ADDR="${VAULT_ADDR:-http://vault:8200}"
KEYS_FILE="/vault/data/init-keys.json"
KEY_NAME="${VAULT_KEY_NAME:-manifold-kek}"

export VAULT_ADDR

echo "Waiting for Vault to be available..."
until vault status -format=json 2>&1 | grep -q '"type"'; do
  sleep 1
done

# Check initialization status
INIT_STATUS=$(vault status -format=json 2>/dev/null || echo '{"initialized":false}')
IS_INITIALIZED=$(echo "$INIT_STATUS" | grep -o '"initialized": *[^,}]*' | sed 's/.*: *//' | tr -d ' ')

if [ "$IS_INITIALIZED" != "true" ]; then
  echo "Vault not initialized. Performing first-time initialization..."
  
  # Initialize with 1 key share and 1 threshold for simplicity
  # In production, use 5 shares with threshold of 3
  vault operator init -key-shares=1 -key-threshold=1 -format=json > "$KEYS_FILE"
  
  echo "Vault initialized. Keys saved to $KEYS_FILE"
  echo "WARNING: Secure these keys! Loss means permanent data loss."
  
  # Extract unseal key and root token using sed (more portable)
  UNSEAL_KEY=$(sed -n 's/.*"unseal_keys_b64":\["\([^"]*\)".*/\1/p' "$KEYS_FILE")
  ROOT_TOKEN=$(sed -n 's/.*"root_token":"\([^"]*\)".*/\1/p' "$KEYS_FILE")
  
  echo "Unsealing Vault with key..."
  echo "$UNSEAL_KEY" | vault operator unseal -
  
  echo "Logging in with root token..."
  export VAULT_TOKEN="$ROOT_TOKEN"
  
  echo "Enabling Transit secrets engine..."
  vault secrets enable transit
  
  echo "Creating encryption key: $KEY_NAME..."
  vault write -f "transit/keys/$KEY_NAME"
  
  # Create a policy for the manifold application
  echo "Creating manifold-transit policy..."
  vault policy write manifold-transit - <<EOF
# Allow encrypt/decrypt operations on the manifold KEK
path "transit/encrypt/$KEY_NAME" {
  capabilities = ["update"]
}

path "transit/decrypt/$KEY_NAME" {
  capabilities = ["update"]
}

# Allow key info lookup
path "transit/keys/$KEY_NAME" {
  capabilities = ["read"]
}
EOF

  # Create a token for the application with the policy
  echo "Creating application token..."
  APP_TOKEN=$(vault token create -policy=manifold-transit -period=768h -format=json | grep -o '"client_token":"[^"]*"' | cut -d'"' -f4)
  
  # Save the app token alongside the init keys
  echo "{\"app_token\":\"$APP_TOKEN\"}" > /vault/data/app-token.json
  
  echo "Application token saved to /vault/data/app-token.json"
  echo ""
  echo "=== INITIALIZATION COMPLETE ==="
  echo "Root token and unseal keys are in: $KEYS_FILE"
  echo "Application token is in: /vault/data/app-token.json"
  echo ""
  echo "For the manifold application, use the app token from app-token.json"
  echo "or set VAULT_TOKEN in your environment."
  
else
  echo "Vault already initialized."
  
  # Check if sealed
  IS_SEALED=$(echo "$INIT_STATUS" | grep -o '"sealed": *[^,}]*' | sed 's/.*: *//' | tr -d ' ')
  
  if [ "$IS_SEALED" = "true" ]; then
    echo "Vault is sealed. Attempting to unseal..."
    
    if [ -f "$KEYS_FILE" ]; then
      UNSEAL_KEY=$(sed -n 's/.*"unseal_keys_b64":\["\([^"]*\)".*/\1/p' "$KEYS_FILE")
      echo "$UNSEAL_KEY" | vault operator unseal -
      echo "Vault unsealed successfully."
    else
      echo "ERROR: No keys file found at $KEYS_FILE"
      echo "Manual unseal required!"
      exit 1
    fi
  else
    echo "Vault is already unsealed."
  fi
  
  # Verify transit engine and key exist
  if [ -f "$KEYS_FILE" ]; then
    ROOT_TOKEN=$(sed -n 's/.*"root_token":"\([^"]*\)".*/\1/p' "$KEYS_FILE")
    export VAULT_TOKEN="$ROOT_TOKEN"
    
    # Check if transit is enabled
    if ! vault secrets list -format=json | grep -q '"transit/"'; then
      echo "Enabling Transit secrets engine..."
      vault secrets enable transit
    fi
    
    # Check if key exists
    if ! vault read "transit/keys/$KEY_NAME" >/dev/null 2>&1; then
      echo "Creating encryption key: $KEY_NAME..."
      vault write -f "transit/keys/$KEY_NAME"
    fi
    
    echo "Transit engine and key '$KEY_NAME' verified."
  fi
fi

echo "Vault initialization script completed."
