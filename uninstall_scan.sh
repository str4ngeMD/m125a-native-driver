#!/bin/bash
# Description: Automated native scan driver uninstaller for HP LaserJet Pro MFP M125a on macOS ARM64.
#              Cleanly removes the scan-go backend and custom ICA driver app.

set -e

# Prevent running as root/sudo
if [ "$EUID" -eq 0 ]; then
    echo "ERROR: Do not run this script as root or with sudo!"
    echo "Please run it as a regular user: ./uninstall_scan.sh"
    echo "The script will ask for your administrator password automatically when removing system files."
    exit 1
fi

BIN_DIR="/Library/Printers/hpcups-str4ngemd/bin"
SCAN_BIN="$BIN_DIR/scan-go"
ICA_APP="/Library/Image Capture/Devices/M125aScanner.app"

echo "=== HP LaserJet Pro MFP M125a Native Scanner Uninstaller ==="

# 1. Remove scan-go backend
if [ -f "$SCAN_BIN" ]; then
    echo "Removing scan-go backend..."
    sudo rm -f "$SCAN_BIN"
fi

# Clean up printer parent directory if empty
sudo rmdir "$BIN_DIR" 2>/dev/null || true
sudo rmdir "/Library/Printers/hpcups-str4ngemd" 2>/dev/null || true

# 2. Remove ICA App
if [ -d "$ICA_APP" ]; then
    echo "Removing M125aScanner ICA App..."
    sudo rm -rf "$ICA_APP"
fi

echo "======================================================"
echo "Scanner Uninstallation complete!"
echo "======================================================"
