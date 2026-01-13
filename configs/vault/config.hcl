# Vault Server Configuration (Production Mode)
# This configuration uses file storage for persistence and disables dev mode.

# Storage backend - file-based for single-node deployments
storage "file" {
  path = "/vault/data"
}

# Listener configuration
listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = true  # Enable TLS in production with proper certificates
}

# API address for client connections
api_addr = "http://0.0.0.0:8200"

# Disable mlock for container environments (use IPC_LOCK capability instead)
disable_mlock = true

# UI is disabled by default in production; enable if needed
ui = false

# Telemetry (optional - uncomment to enable)
# telemetry {
#   prometheus_retention_time = "30s"
#   disable_hostname          = true
# }
