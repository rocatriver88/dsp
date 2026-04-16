#!/usr/bin/env bash
# cleanup-legacy-uploads.sh
#
# One-time migration: moves any files still sitting in the legacy uploads/
# directory into var/uploads/ (the only directory now served by the upload
# file server). Run from the project root.
#
# Usage:  bash scripts/cleanup-legacy-uploads.sh
#
# Safe to run multiple times — skips files that already exist at the
# destination and never deletes the source until the copy is confirmed.

set -euo pipefail

LEGACY_DIR="uploads"
TARGET_DIR="var/uploads"

if [ ! -d "$LEGACY_DIR" ]; then
    echo "No legacy $LEGACY_DIR/ directory found — nothing to do."
    exit 0
fi

mkdir -p "$TARGET_DIR"

count=0
skipped=0

for src in "$LEGACY_DIR"/*; do
    [ -f "$src" ] || continue
    fname="$(basename "$src")"
    dst="$TARGET_DIR/$fname"

    if [ -e "$dst" ]; then
        echo "SKIP  $fname (already exists in $TARGET_DIR/)"
        skipped=$((skipped + 1))
        continue
    fi

    cp -- "$src" "$dst"
    rm -- "$src"
    echo "MOVED $fname -> $TARGET_DIR/"
    count=$((count + 1))
done

echo ""
echo "Done. Moved $count file(s), skipped $skipped."

# Remove the legacy directory if it's now empty.
if [ -d "$LEGACY_DIR" ] && [ -z "$(ls -A "$LEGACY_DIR")" ]; then
    rmdir "$LEGACY_DIR"
    echo "Removed empty $LEGACY_DIR/ directory."
fi
