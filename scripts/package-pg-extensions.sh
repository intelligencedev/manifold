#!/usr/bin/env bash
set -euo pipefail

# Package PostgreSQL extensions for embedded-postgres distribution.
#
# Produces tar.gz archives compatible with the manifold embedded-postgres
# extension installer. Expected layout inside the archive:
#
#   lib/                 → PG extension shared libraries
#   deps/                → Non-PG shared library dependencies
#   share/extension/     → .control + .sql files
#
# Usage:
#   ./package-pg-extensions.sh --ext pgvector --pg-major 18 [--output-dir dist/]
#   ./package-pg-extensions.sh --ext postgis  --pg-major 18 [--output-dir dist/]
#   ./package-pg-extensions.sh --all --pg-major 18
#
# Requires: Homebrew (macOS) or apt packages (Linux) with the target
# extension already installed for the matching PG major version.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/../dist/pgext"
PG_MAJOR=""
EXT_NAME=""
PACK_ALL=false

usage() {
    echo "Usage: $0 --ext <name> --pg-major <N> [--output-dir <dir>]"
    echo "       $0 --all --pg-major <N> [--output-dir <dir>]"
    echo ""
    echo "Supported extensions: pgvector, postgis, pgrouting"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --ext)       EXT_NAME="$2"; shift 2 ;;
        --pg-major)  PG_MAJOR="$2"; shift 2 ;;
        --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
        --all)       PACK_ALL=true; shift ;;
        -h|--help)   usage ;;
        *)           echo "Unknown option: $1"; usage ;;
    esac
done

[[ -z "$PG_MAJOR" ]] && { echo "Error: --pg-major is required"; usage; }

# Detect platform.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  ARCH_TAG="amd64" ;;
    aarch64|arm64) ARCH_TAG="arm64v8" ;;
    *) echo "Unsupported arch: $ARCH"; exit 1 ;;
esac

mkdir -p "$OUTPUT_DIR"

# ---------------------------------------------------------------------------
# Helper: detect source paths for an extension
# ---------------------------------------------------------------------------
detect_paths() {
    local ext="$1"
    local brew_pkg="$ext"

    if [[ "$OS" == "darwin" ]]; then
        # Try Apple Silicon, then Intel
        for prefix in "/opt/homebrew/opt/$brew_pkg" "/usr/local/opt/$brew_pkg"; do
            if [[ -d "$prefix" ]]; then
                EXT_LIB_DIR="$prefix/lib/postgresql@${PG_MAJOR}"
                EXT_SHARE_DIR="$prefix/share/postgresql@${PG_MAJOR}/extension"
                return 0
            fi
        done
        echo "Error: Homebrew package '$brew_pkg' not found" >&2
        return 1
    elif [[ "$OS" == "linux" ]]; then
        EXT_LIB_DIR="/usr/lib/postgresql/${PG_MAJOR}/lib"
        EXT_SHARE_DIR="/usr/share/postgresql/${PG_MAJOR}/extension"
        if [[ ! -d "$EXT_LIB_DIR" ]]; then
            echo "Error: $EXT_LIB_DIR not found. Install postgresql-${PG_MAJOR}-${ext}" >&2
            return 1
        fi
        return 0
    else
        echo "Error: Unsupported OS '$OS'" >&2
        return 1
    fi
}

# ---------------------------------------------------------------------------
# Helper: get lib file names for an extension
# ---------------------------------------------------------------------------
lib_files_for() {
    local ext="$1"
    local suffix=".so"
    [[ "$OS" == "darwin" ]] && suffix=".dylib"

    case "$ext" in
        pgvector)   echo "vector${suffix}" ;;
        postgis)    echo "postgis-3${suffix} postgis_raster-3${suffix} postgis_topology-3${suffix} address_standardizer-3${suffix}" ;;
        pgrouting)  echo "libpgrouting-4.0${suffix}" ;;
        *)          echo "${ext}${suffix}" ;;
    esac
}

# ---------------------------------------------------------------------------
# Helper: get share file prefixes for an extension
# ---------------------------------------------------------------------------
share_prefixes_for() {
    local ext="$1"
    case "$ext" in
        pgvector)   echo "vector" ;;
        postgis)    echo "postgis address_standardizer postgis_raster postgis_topology" ;;
        pgrouting)  echo "pgrouting" ;;
        *)          echo "$ext" ;;
    esac
}

# ---------------------------------------------------------------------------
# Helper: get extension version from .control file
# ---------------------------------------------------------------------------
ext_version() {
    local share_dir="$1"
    local prefix="$2"
    local control="${share_dir}/${prefix}.control"
    if [[ -f "$control" ]]; then
        # macOS-compatible: extract version from default_version = 'X.Y.Z'
        sed -n "s/^default_version[[:space:]]*=[[:space:]]*'\([^']*\)'.*/\1/p" "$control" 2>/dev/null || echo "unknown"
    else
        echo "unknown"
    fi
}

# ---------------------------------------------------------------------------
# macOS: bundle non-system dylib dependencies
# ---------------------------------------------------------------------------
bundle_macos_deps() {
    local staging="$1"
    local lib_dir="${staging}/lib"
    local deps_dir="${staging}/deps"

    mkdir -p "$deps_dir"

    # Collect non-system dependency dylibs for all extension modules.
    for dylib in "${lib_dir}"/*.dylib; do
        [[ -f "$dylib" ]] || continue
        otool -L "$dylib" 2>/dev/null | tail -n +2 | awk '{print $1}' | while read -r dep; do
            # Skip system libraries and the dylib itself.
            case "$dep" in
                /usr/lib/*|/System/*|@*) continue ;;
            esac
            local base
            base="$(basename "$dep")"
            if [[ ! -f "${deps_dir}/${base}" ]]; then
                cp "$dep" "${deps_dir}/${base}" 2>/dev/null || true
            fi
            # Rewrite the reference to @loader_path/../lib/<base> so PG can
            # find it when the deps are placed in binariesPath/lib/.
            install_name_tool -change "$dep" "@loader_path/../${base}" "$dylib" 2>/dev/null || true
        done
    done

    # Fix the deps' own inter-dependencies.
    for dep_dylib in "${deps_dir}"/*.dylib; do
        [[ -f "$dep_dylib" ]] || continue
        local dep_base
        dep_base="$(basename "$dep_dylib")"
        install_name_tool -id "@loader_path/${dep_base}" "$dep_dylib" 2>/dev/null || true
        otool -L "$dep_dylib" 2>/dev/null | tail -n +2 | awk '{print $1}' | while read -r subdep; do
            case "$subdep" in
                /usr/lib/*|/System/*|@*) continue ;;
            esac
            local sub_base
            sub_base="$(basename "$subdep")"
            if [[ ! -f "${deps_dir}/${sub_base}" ]]; then
                cp "$subdep" "${deps_dir}/${sub_base}" 2>/dev/null || true
            fi
            install_name_tool -change "$subdep" "@loader_path/${sub_base}" "$dep_dylib" 2>/dev/null || true
        done
    done

    # Remove deps dir if nothing was bundled.
    if [[ -z "$(ls -A "$deps_dir" 2>/dev/null)" ]]; then
        rmdir "$deps_dir"
    fi
}

# ---------------------------------------------------------------------------
# Package one extension
# ---------------------------------------------------------------------------
package_extension() {
    local ext="$1"
    echo "==> Packaging $ext for PG ${PG_MAJOR} (${OS}/${ARCH_TAG})..."

    detect_paths "$ext" || return 1

    local staging
    staging="$(mktemp -d)"
    trap "rm -rf '$staging'" RETURN

    mkdir -p "${staging}/lib" "${staging}/share/extension"

    # Copy shared libraries.
    local found=false
    for f in $(lib_files_for "$ext"); do
        local src="${EXT_LIB_DIR}/${f}"
        if [[ -f "$src" ]]; then
            cp "$src" "${staging}/lib/${f}"
            found=true
        fi
    done

    if [[ "$found" != "true" ]]; then
        echo "  ERROR: No library files found for $ext in $EXT_LIB_DIR" >&2
        return 1
    fi

    # Copy share/extension files.
    for prefix in $(share_prefixes_for "$ext"); do
        for f in "${EXT_SHARE_DIR}/${prefix}"*; do
            [[ -f "$f" ]] && cp "$f" "${staging}/share/extension/"
        done
    done

    # On macOS, bundle non-system dependencies and fix paths.
    if [[ "$OS" == "darwin" ]]; then
        bundle_macos_deps "$staging"
    fi

    # Determine version from the primary control file.
    local primary_prefix
    primary_prefix="$(share_prefixes_for "$ext" | awk '{print $1}')"
    local version
    version="$(ext_version "${staging}/share/extension" "$primary_prefix")"

    local archive_name="${ext}-${version}-pg${PG_MAJOR}-${OS}-${ARCH_TAG}.tar.gz"
    tar -czf "${OUTPUT_DIR}/${archive_name}" -C "$staging" .

    echo "  Created: ${OUTPUT_DIR}/${archive_name}"
    echo "  Version: ${version}"
    echo "  Files:"
    tar -tzf "${OUTPUT_DIR}/${archive_name}" | head -20
    local count
    count="$(tar -tzf "${OUTPUT_DIR}/${archive_name}" | wc -l | tr -d ' ')"
    [[ "$count" -gt 20 ]] && echo "  ... and $((count - 20)) more"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
if [[ "$PACK_ALL" == "true" ]]; then
    for ext in pgvector postgis pgrouting; do
        package_extension "$ext" || echo "  WARN: skipped $ext"
    done
elif [[ -n "$EXT_NAME" ]]; then
    package_extension "$EXT_NAME"
else
    usage
fi

echo ""
echo "Done. Extension packages are in: $OUTPUT_DIR"
