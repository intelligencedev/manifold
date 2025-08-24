#!/bin/bash

# Script to run whisper-go with proper library paths (based on whisper.cpp/bindings/go/Makefile)
WHISPER_ROOT="/Users/art/Documents/code/whisper.cpp"
BUILD_DIR="build_go"
INCLUDE_PATH="${WHISPER_ROOT}/include:${WHISPER_ROOT}/ggml/include"
LIBRARY_PATH="${WHISPER_ROOT}/${BUILD_DIR}/src:${WHISPER_ROOT}/${BUILD_DIR}/ggml/src:${WHISPER_ROOT}/${BUILD_DIR}/ggml/src/ggml-blas:${WHISPER_ROOT}/${BUILD_DIR}/ggml/src/ggml-metal"
GGML_METAL_PATH_RESOURCES="${WHISPER_ROOT}"

export C_INCLUDE_PATH="${INCLUDE_PATH}"
export LIBRARY_PATH="${LIBRARY_PATH}"
export GGML_METAL_PATH_RESOURCES="${GGML_METAL_PATH_RESOURCES}"

# Run the whisper-go program with any arguments passed to this script
go run ./cmd/whisper-go/main.go "$@"
