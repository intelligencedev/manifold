#!/bin/bash

set -e # Exit immediately if a command exits with a non-zero status.

# --- Configuration ---
DEP_INSTALL_TIMEOUT=120
CODE_EXEC_TIMEOUT=30
TEMP_DIR_BASE="/app/run"

# --- Input Validation ---
if [ -z "$1" ]; then
  jq -n '{return_code: -1, stdout: "", stderr: "Error: Language (python|go|node) argument missing."}'
  exit 1
fi
LANGUAGE=$1

# --- Read JSON from stdin ---
JSON_INPUT=$(cat)
if [ -z "$JSON_INPUT" ]; then
  jq -n '{return_code: -1, stdout: "", stderr: "Error: No JSON input received on stdin."}'
  exit 1
fi

# --- Parse JSON ---
CODE=$(echo "$JSON_INPUT" | jq -r '.code // ""')
readarray -t DEPENDENCIES < <(echo "$JSON_INPUT" | jq -r '.dependencies[]? // empty')

if [ -z "$CODE" ]; then
    jq -n '{return_code: -1, stdout: "", stderr: "Error: '\''code'\'' field missing or empty in JSON input."}'
    exit 1
fi

# --- Prepare Execution Environment ---
RUN_DIR=$(mktemp -d -p "$TEMP_DIR_BASE" "sandbox_XXXXXXXXXX")
trap 'rm -rf "$RUN_DIR"' EXIT
cd "$RUN_DIR"

STDOUT_FILE="sandbox_stdout.log"
STDERR_FILE="sandbox_stderr.log"
RETURN_CODE=0

# --- Output Formatting ---
finalize_output() {
  # --- Debug: Print raw files to container stderr ---
  # Use head/tail or similar if files are huge, but for typical output cat is fine.
  echo "--- DEBUG: Raw STDOUT_FILE content ---" > /dev/stderr
  cat "$STDOUT_FILE" > /dev/stderr
  echo "--- DEBUG: End Raw STDOUT_FILE ---" > /dev/stderr
  echo "--- DEBUG: Raw STDERR_FILE content ---" > /dev/stderr
  cat "$STDERR_FILE" > /dev/stderr
  echo "--- DEBUG: End Raw STDERR_FILE ---" > /dev/stderr
  # --- End Debug ---

  # --- Enhanced Sanitization - Pipe directly from file ---
  # 1. Remove ANSI escape codes
  # 2. Remove other non-printable control characters (ASCII 0-8, 11-31, 127)
  # Use process substitution <() to feed files directly to the pipeline
  local stdout_cleaned=$(sed 's/\x1b\[[0-9;]*[a-zA-Z]//g' < "$STDOUT_FILE" | tr -d '\000-\010\013-\037\177')
  local stderr_cleaned=$(sed 's/\x1b\[[0-9;]*[a-zA-Z]//g' < "$STDERR_FILE" | tr -d '\000-\010\013-\037\177')
  # --- End Enhanced Sanitization ---

  # --- Debug: Print cleaned strings to container stderr ---
  echo "--- DEBUG: Cleaned STDOUT content ---" > /dev/stderr
  echo "$stdout_cleaned" > /dev/stderr
  echo "--- DEBUG: End Cleaned STDOUT ---" > /dev/stderr
  echo "--- DEBUG: Cleaned STDERR content ---" > /dev/stderr
  echo "$stderr_cleaned" > /dev/stderr
  echo "--- DEBUG: End Cleaned STDERR ---" > /dev/stderr
  # --- End Debug ---


  # Ensure RETURN_CODE is treated as a number
  if ! [[ "$RETURN_CODE" =~ ^-?[0-9]+$ ]]; then
      # Log to the *original* stderr file
      echo "Warning: Invalid non-numeric return code detected ('$RETURN_CODE'), setting to -1." >> "$STDERR_FILE"
      # Also log to container stderr for immediate visibility
      echo "Warning: Invalid non-numeric return code detected ('$RETURN_CODE'), setting to -1." > /dev/stderr
      RETURN_CODE=-1
      # Re-clean stderr since we added a message to the file
      # Note: This re-cleaning might slightly alter the debug output vs final output if this warning triggers.
      stderr_cleaned=$(sed 's/\x1b\[[0-9;]*[a-zA-Z]//g' < "$STDERR_FILE" | tr -d '\000-\010\013-\037\177')
  fi

  # Output the final JSON result
  # If jq still fails, the debug output above should show the problematic character(s)
  jq -n \
    --argjson rc "$RETURN_CODE" \
    --arg out "$stdout_cleaned" \
    --arg err "$stderr_cleaned" \
    '{return_code: $rc, stdout: $out, stderr: $err}'
}


# --- Language-Specific Handling ---
install_and_run() {
  local lang=$1
  local install_cmd_template=$2
  local run_cmd_template=$3
  local code_filename=$4
  local install_failed=0

  # 1. Install Dependencies
  if [ ${#DEPENDENCIES[@]} -gt 0 ]; then
    echo "--- Installing dependencies (${lang}) ---" >> "$STDERR_FILE"
    local combined_install_cmd=""
    for dep in "${DEPENDENCIES[@]}"; do
       dep_sanitized=$(echo "$dep" | sed 's/[^a-zA-Z0-9._\/@-]//g')
       if [ -z "$dep_sanitized" ]; then
           echo "Skipping potentially unsafe or empty dependency: $dep" >> "$STDERR_FILE"
           continue
       fi
       eval "local cmd=\"$install_cmd_template\""
       combined_install_cmd+="$cmd && "
    done
    combined_install_cmd+="true"

    timeout "$DEP_INSTALL_TIMEOUT" bash -c "$combined_install_cmd" >> "$STDOUT_FILE" 2>> "$STDERR_FILE" || {
      local exit_status=$?
      if [ $exit_status -eq 124 ]; then
          echo "Error: Dependency installation timed out after ${DEP_INSTALL_TIMEOUT}s." >> "$STDERR_FILE"
      else
          echo "Error: Dependency installation failed with exit code $exit_status." >> "$STDERR_FILE"
      fi
      install_failed=1
      RETURN_CODE=${exit_status}
    }

    if [ $install_failed -eq 1 ]; then
      echo "--- Dependency installation failed, skipping code execution (${lang}) ---" >> "$STDERR_FILE"
      return 1 # Signal failure
    fi
    echo "--- Dependency installation finished (${lang}) ---" >> "$STDERR_FILE"
  fi

  # 2. Write Code
  echo "$CODE" > "$code_filename"

  # 3. Run Code
  echo "--- Running code (${lang}) ---" >> "$STDERR_FILE"
  eval "local run_cmd=\"$run_cmd_template\""

  timeout "$CODE_EXEC_TIMEOUT" bash -c "$run_cmd" >> "$STDOUT_FILE" 2>> "$STDERR_FILE"
  local run_exit_status=$?
  RETURN_CODE=$run_exit_status

  if [ $RETURN_CODE -eq 124 ]; then
      echo -e "\nError: Code execution timed out after $CODE_EXEC_TIMEOUT seconds." >> "$STDERR_FILE"
  elif [ $RETURN_CODE -ne 0 ]; then
      :
  fi
   echo "--- Code execution finished (${lang}) ---" >> "$STDERR_FILE"
   return 0 # Signal success
}


# --- Main Execution Logic ---
case "$LANGUAGE" in
  "python")
    python3 -m venv venv >> "$STDOUT_FILE" 2>> "$STDERR_FILE" || { RETURN_CODE=$?; echo "Failed to create Python venv" >> "$STDERR_FILE"; finalize_output; exit 0; }
    source venv/bin/activate
    pip install --disable-pip-version-check --no-cache-dir --upgrade pip wheel >> "$STDOUT_FILE" 2>> "$STDERR_FILE" || echo "Warning: pip/wheel upgrade failed." >> "$STDERR_FILE"
    install_cmd_template='pip install --no-cache-dir \"$dep_sanitized\"'
    run_cmd_template='python user_code.py'
    install_and_run "python" "$install_cmd_template" "$run_cmd_template" "user_code.py"
    ;;

  "go")
    export GOPATH="$RUN_DIR/.go"
    export PATH=$PATH:$GOPATH/bin
    mkdir -p "$GOPATH"
    go mod init sandbox >> "$STDOUT_FILE" 2>> "$STDERR_FILE" || { RETURN_CODE=$?; echo "Error: go mod init failed." >> "$STDERR_FILE"; finalize_output; exit 0; }
    install_cmd_template='go get \"$dep_sanitized\"'
    run_cmd_template='go run main.go'
    install_and_run "go" "$install_cmd_template" "$run_cmd_template" "main.go"
    ;;

  "node")
    npm init -y --loglevel=error >> "$STDOUT_FILE" 2>> "$STDERR_FILE" || { RETURN_CODE=$?; echo "Error: npm init failed." >> "$STDERR_FILE"; finalize_output; exit 0; }
    install_cmd_template='npm install --loglevel=error --no-fund --no-audit \"$dep_sanitized\"'
    run_cmd_template='node script.js'
    install_and_run "node" "$install_cmd_template" "$run_cmd_template" "script.js"
    ;;

  *)
    jq -n --arg lang "$LANGUAGE" '{return_code: -1, stdout: "", stderr: "Error: Unsupported language '\''\($lang)'\''. Use '\''python'\'', '\''go'\'', or '\''node'\''."}'
    exit 1
    ;;
esac

# --- Finalize and Output ---
finalize_output

exit 0