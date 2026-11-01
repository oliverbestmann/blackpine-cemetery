#!/bin/sh
# Builds the web version with the TinyGo fork into dist/wasm-tinygo. Run inside the nix devShell:
# nix develop ../nix -c ./build-wasm-tinygo.sh

set -e

cd "$(dirname "$0")"

TOOLS=${TOOLS:-/dev/shm/tinygo-tools}
nix build --out-link "$TOOLS/tinygo" github:oliverbestmann/tinygo/dev
nix build --out-link "$TOOLS/binaryen" --inputs-from github:oliverbestmann/tinygo/dev nixpkgs#binaryen

export PATH="$TOOLS/tinygo/bin:$TOOLS/binaryen/bin:$PATH"
export XDG_CACHE_HOME=${XDG_CACHE_HOME:-$TOOLS/cache}

# the devShell sets GOFLAGS for the Go toolchain, TinyGo does not understand them
unset GOFLAGS

# TinyGo does not implement simd/archsimd, which box3d-go uses with goexperiment.simd
export GOEXPERIMENT=nosimd

# byke-cli always writes to dist/wasm, keep the Go build there
rm -rf dist/wasm-tinygo
if [ -d dist/wasm ]; then mv dist/wasm dist/wasm-go.tmp; fi
trap 'if [ -d dist/wasm-go.tmp ]; then rm -rf dist/wasm; mv dist/wasm-go.tmp dist/wasm; fi' EXIT

go run github.com/oliverbestmann/byke-cli build-web --tinygo "$@"
mv dist/wasm dist/wasm-tinygo
