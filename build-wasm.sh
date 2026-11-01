#!/bin/sh
# Records a fresh default.pgo with an offscreen native run, then builds the wasm version with it.
# Usage: ./build-wasm.sh [build-web flags], e.g. --clean. PGO_DURATION sets the offscreen run time.

set -e

cd "$(dirname "$0")"

# run byke-cli from the workspace, so it is never an outdated installed binary
go run github.com/oliverbestmann/byke-cli pgo --duration "${PGO_DURATION:-30s}"
go run github.com/oliverbestmann/byke-cli build-web "$@"
