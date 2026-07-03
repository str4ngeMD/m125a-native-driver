#!/bin/bash
# Description: Automated native driver uninstaller for HP LaserJet Pro MFP M125a on macOS ARM64.
#              Cleanly removes the print filter binary and custom PPD.

set -e

# Prevent running as root/sudo
if [ "$EUID" -eq 0 ]; then
    echo "ERROR: Do not run this script as root or with sudo!"
    echo "Please run it as a regular user: ./uninstall.sh"
    echo "The script will ask for your administrator password automatically when removing system files."
    exit 1
fi

FILTER_DIR="/Library/Printers/hpcups-str4ngemd/filter"
FILTER_FILE="$FILTER_DIR/hpcups-native"
PPD_TARGET="/Library/Printers/PPDs/Contents/Resources/HP_LaserJet_Pro_MFP_M125a_Native.ppd"

echo "=== HP LaserJet Pro MFP M125a Native Driver Uninstaller ==="

# 1. Remove print filter
if [ -f "$FILTER_FILE" ]; then
    echo "Removing native filter binary..."
    sudo rm -f "$FILTER_FILE"
    # Attempt to clean up directory if empty
    sudo rmdir "$FILTER_DIR" 2>/dev/null || true
    sudo rmdir "/Library/Printers/hpcups-str4ngemd" 2>/dev/null || true
fi

# 2. Remove PPD
if [ -f "$PPD_TARGET" ]; then
    echo "Removing custom PPD file..."
    sudo rm -f "$PPD_TARGET"
fi

echo "======================================================"
echo "Uninstallation complete!"
echo "======================================================"
