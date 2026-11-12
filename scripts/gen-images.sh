#!/usr/bin/env bash
# Regenerates the plugin's static PNG images from the Go renderer. Requires rsvg-convert.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$root"
go run ./tools/genassets "$tmp"
img="$root/com.marco.weather.sdPlugin/images"
mkdir -p "$img"
rsvg-convert -w 128 -h 128 "$tmp/icon.svg" -o "$img/plugin.png"
rsvg-convert -w 48 -h 48 "$tmp/icon.svg" -o "$img/category.png"
rsvg-convert -w 80 -h 80 "$tmp/icon.svg" -o "$img/action.png"
rsvg-convert -w 144 -h 144 "$tmp/key.svg" -o "$img/key.png"
rsvg-convert -w 320 -h 130 "$tmp/knob.svg" -o "$img/knob.png"
echo "Images written to $img"
