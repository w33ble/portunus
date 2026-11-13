#!/usr/bin/env bash
# Renders the tile-based icon set from build/icon/master-1024.png (transparent artwork).
# The favicon (frontend/public) and README art (docs/icon-512.png) are the designer's files as-is.
# Needs ImageMagick 7 (`magick`). Then run `mise run icons:platform` for .icns/.ico.
# The macOS 26 icon (build/appicon.icon) uses build/icon/portunus.svg directly.
set -euo pipefail

cd "$(dirname "$0")/../.."
SRC=build/icon/master-1024.png
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# Apple's macOS icon grid: an 824px rounded-rect plate centered on a 1024px canvas.
PLATE=824 RADIUS=185 OFFSET=100 ART=640

magick -size ${PLATE}x${PLATE} xc:none -fill white \
  -draw "roundrectangle 0,0 $((PLATE - 1)),$((PLATE - 1)) $RADIUS,$RADIUS" "$TMP/mask.png"

magick -size ${PLATE}x${PLATE} gradient:'#ffffff-#e4ecf5' "$TMP/mask.png" \
  -compose CopyOpacity -composite "$TMP/plate.png"

# Artwork sits slightly below center: the pediment is visually heavier than the base.
magick "$SRC" -trim +repage -resize ${ART}x${ART} "$TMP/art.png"
magick "$TMP/plate.png" "$TMP/art.png" -gravity center -geometry +0+10 \
  -compose Over -composite "$TMP/tile.png"

magick "$TMP/mask.png" -background black -shadow 32x14+0+0 "$TMP/shadow.png"
SHADOW_PAD=$(( 2 * 14 ))
magick -size 1024x1024 xc:none \
  "$TMP/shadow.png" -geometry +$((OFFSET - SHADOW_PAD))+$((OFFSET - SHADOW_PAD + 12)) -composite \
  "$TMP/tile.png" -geometry +${OFFSET}+${OFFSET} -composite -depth 8 \
  build/appicon.png

# DMG: the disk image file uses the app tile; the window background is a plain gradient.
cp build/appicon.png build/darwin/dmg-file-icon.png
magick -size 540x380 -define gradient:direction=SouthEast gradient:'#173859-#f5f8fc' -depth 8 \
  build/darwin/dmg-background.png

# Linux desktop icon: same tile as macOS, at the hicolor 512px size.
magick build/appicon.png -resize 512x512 -depth 8 build/linux/portunus-512.png
