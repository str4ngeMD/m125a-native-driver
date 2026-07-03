#!/bin/bash
# Description: Automated native driver installer for HP LaserJet Pro MFP M125a on macOS ARM64.
#              Copies the native print filter, custom PPD, code-signs, and secures permissions.
#              Run as normal user (the script will prompt for sudo when copying system files).

set -e

# Prevent running as root/sudo
if [ "$EUID" -eq 0 ]; then
    echo "ERROR: Do not run this script as root or with sudo!"
    echo "Please run it as a regular user: ./install.sh"
    echo "The script will ask for your administrator password automatically when writing to system folders."
    exit 1
fi

# Target folders
FILTER_DIR="/Library/Printers/hpcups-str4ngemd/filter"
PPD_TARGET="/Library/Printers/PPDs/Contents/Resources/HP_LaserJet_Pro_MFP_M125a_Native.ppd"

echo "=== HP LaserJet Pro MFP M125a Native Driver Installer ==="

# 1. Create target system directories
echo "Creating system printer directories at $FILTER_DIR..."
sudo mkdir -p "$FILTER_DIR"

# 2. Copy the native filter binary
echo "Installing native C++ filter binary..."
if [ ! -f hpcups-native ]; then
    echo "ERROR: hpcups-native binary not found in current folder! Please make sure you are running this from the repository folder."
    exit 1
fi

# Strip quarantine attribute if downloaded from the web
xattr -d com.apple.quarantine hpcups-native 2>/dev/null || true

sudo cp hpcups-native "$FILTER_DIR/hpcups-native"

# 3. Codesign and secure the filter binary permissions
echo "Codesigning and setting permissions for the filter..."
sudo codesign --force --sign - "$FILTER_DIR/hpcups-native"
sudo chown -R root:wheel "$FILTER_DIR"
sudo chmod 0555 "$FILTER_DIR/hpcups-native"

# 4. Copy the custom PPD file
echo "Installing custom PPD definition..."
if [ ! -f HP_LaserJet_Pro_MFP_M125a.ppd ]; then
    echo "ERROR: HP_LaserJet_Pro_MFP_M125a.ppd not found in current folder!"
    exit 1
fi
sudo cp HP_LaserJet_Pro_MFP_M125a.ppd "$PPD_TARGET"
sudo chown root:wheel "$PPD_TARGET"
sudo chmod 0644 "$PPD_TARGET"

echo "======================================================"
echo "Installation complete!"
echo "You can now connect the printer via USB."
echo "macOS will auto-discover it and create the print queue!"
echo "======================================================"
