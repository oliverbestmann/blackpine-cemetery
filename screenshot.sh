#!/bin/sh
# Renders the game offscreen with the software Vulkan driver and saves one frame.
# Usage: ./screenshot.sh [out.png] [frame]. Run inside the nix devShell: nix develop ../nix -c ./screenshot.sh
set -e
cd "$(dirname "$0")"

MESA=${MESA:-/dev/shm/mesa}
if [ ! -e "$MESA" ]; then
  nix build --out-link "$MESA" --inputs-from ../nix nixpkgs#mesa
fi

export GOEXPERIMENT=simd
export WGPU_FORCE_FALLBACK_ADAPTER=1
export VK_DRIVER_FILES=$MESA/share/vulkan/icd.d/lvp_icd.aarch64.json
export LP_NUM_THREADS=0
export ZOMBIES_SCREENSHOT=${1:-/tmp/zombies.png}
export ZOMBIES_SCREENSHOT_FRAME=${2:-60}

exec go run --tags fakeClock .
