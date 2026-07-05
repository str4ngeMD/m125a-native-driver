#!/bin/bash
# Description: Automated native scan driver installer for HP LaserJet Pro MFP M125a on macOS ARM64.
#              Compiles the scan-go backend, builds the Xcode ICA wrapper, registers code-signing.
#              Run as normal user (the script will prompt for sudo when copying system files).

set -e

# Prevent running as root/sudo
if [ "$EUID" -eq 0 ]; then
    echo "ERROR: Do not run this script as root or with sudo!"
    echo "Please run it as a regular user: ./install_scan.sh"
    echo "The script will ask for your administrator password automatically when writing to system folders."
    exit 1
fi

BIN_DIR="/Library/Printers/hpcups-str4ngemd/bin"
SCAN_BIN="$BIN_DIR/scan-go"
ICA_APP="/Library/Image Capture/Devices/M125aScanner.app"

echo "=== HP LaserJet Pro MFP M125a Native Scanner Installer ==="

# 1. Build components
echo "Building scan-go backend..."
if [ ! -d scan_go ]; then
    echo "ERROR: scan_go directory not found! Make sure you are in the repository directory."
    exit 1
fi
(cd scan_go && chmod +x build.sh && ./build.sh)

echo "Building Apple ICA Driver (M125aScanner)..."
if [ ! -d scan_ica ]; then
    echo "ERROR: scan_ica directory not found! Make sure you are in the repository directory."
    exit 1
fi

# Clean and build the Xcode target for Release
xcodebuild -project scan_ica/VirtualScanner.xcodeproj -configuration Release -target VirtualScanner OBJROOT=build SYMROOT=build > /dev/null

# 2. Create target system directories
echo "Creating system directories at $BIN_DIR..."
sudo mkdir -p "$BIN_DIR"

# 3. Copy scan-go binary
echo "Installing scan-go backend..."
sudo cp scan_go/scan-go "$SCAN_BIN"

# 4. Codesign and secure scanner binary
echo "Codesigning and setting permissions for scan-go..."
sudo codesign --force --sign - "$SCAN_BIN"
sudo chmod 0555 "$SCAN_BIN"
sudo chown -R root:wheel "$BIN_DIR"

# 5. Copy and sign the ICA App
echo "Installing M125aScanner ICA App..."
sudo rm -rf "$ICA_APP"
sudo cp -R scan_ica/build/Release/VirtualScanner.app "$ICA_APP"
sudo xattr -cr "$ICA_APP"
sudo codesign --force --deep --sign - "$ICA_APP"
sudo chown -R root:wheel "$ICA_APP"

echo "======================================================"
echo "Scanner Installation complete!"
echo "You can now connect the scanner via USB."
echo "Open Image Capture.app to check the scanner."
echo "======================================================"
