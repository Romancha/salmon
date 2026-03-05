#!/bin/bash
# create-dmg.sh — Creates a .dmg disk image for SalmonRun.app distribution.
#
# Usage: ./tools/create-dmg.sh <path-to-SalmonRun.app> <output.dmg>
#
# Creates a compressed read-only DMG containing SalmonRun.app and
# a symlink to /Applications for drag-install.

set -euo pipefail

if [ $# -ne 2 ]; then
    echo "Usage: $0 <path-to-SalmonRun.app> <output.dmg>" >&2
    exit 1
fi

APP_PATH="$1"
DMG_OUTPUT="$2"

if [ ! -d "$APP_PATH" ]; then
    echo "Error: $APP_PATH does not exist or is not a directory" >&2
    exit 1
fi

APP_NAME=$(basename "$APP_PATH")
VOLUME_NAME="SalmonRun"

# Create temporary directory for DMG contents
STAGING_DIR=$(mktemp -d)
trap 'rm -rf "$STAGING_DIR" "${STAGING_DIR}.dmg" 2>/dev/null' EXIT

echo "Staging DMG contents..."
cp -R "$APP_PATH" "$STAGING_DIR/$APP_NAME"
ln -s /Applications "$STAGING_DIR/Applications"

# Remove existing output if present
rm -f "$DMG_OUTPUT"

# Create writable DMG from staging folder
echo "Creating DMG..."
hdiutil create \
    -volname "$VOLUME_NAME" \
    -srcfolder "$STAGING_DIR" \
    -format UDRW \
    -fs HFS+ \
    "${STAGING_DIR}.dmg"

# Configure Finder window layout (best-effort, may fail in CI/headless)
echo "Configuring DMG window layout..."
MOUNT_OUTPUT=$(hdiutil attach -readwrite -noverify "${STAGING_DIR}.dmg" 2>&1) || true
MOUNT_POINT=$(echo "$MOUNT_OUTPUT" | grep -E 'Apple_HFS' | sed 's/.*Apple_HFS[[:space:]]*//')

if [ -n "$MOUNT_POINT" ] && [ -d "$MOUNT_POINT" ]; then
    osascript <<APPLESCRIPT 2>/dev/null || echo "Note: Finder layout skipped (headless environment)"
tell application "Finder"
    tell disk "$VOLUME_NAME"
        open
        set current view of container window to icon view
        set toolbar visible of container window to false
        set statusbar visible of container window to false
        set the bounds of container window to {400, 200, 900, 480}
        set viewOptions to the icon view options of container window
        set arrangement of viewOptions to not arranged
        set icon size of viewOptions to 80
        set position of item "$APP_NAME" of container window to {125, 140}
        set position of item "Applications" of container window to {375, 140}
        close
    end tell
end tell
APPLESCRIPT
    sync
    hdiutil detach "$MOUNT_POINT" -force 2>/dev/null || true
else
    echo "Note: Could not mount DMG for layout (headless environment)"
fi

# Convert to compressed read-only DMG
echo "Converting to compressed read-only DMG..."
hdiutil convert "${STAGING_DIR}.dmg" \
    -format UDZO \
    -imagekey zlib-level=9 \
    -o "$DMG_OUTPUT"

echo "Created: $DMG_OUTPUT"
echo "Size: $(du -h "$DMG_OUTPUT" | cut -f1)"
