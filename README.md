# HP LaserJet Pro MFP M125a Native macOS ARM64 Driver

This repository contains the drivers, setup scripts, and implementation guides to enable **100% native Apple Silicon (ARM64)** printing and scanning for the HP LaserJet Pro MFP M125a on macOS without Rosetta 2, security bypasses, or SIP modifications.

---

## Part 1: Native Printing Setup (CUPS Driver)

Because the M125a uses `hbpl1` (HP's flavor of PCLm raster printing), it requires HPLIP's open-source `hpcups` filter. We provide a **pre-compiled self-contained ARM64 binary** (`hpcups-native`) statically linked with `libjpeg.a` so it runs natively inside the macOS CUPS sandbox out-of-the-box.

### Option A: Quick Start (Turnkey Installation)

If you trust the pre-compiled binary included in the root of this repository, you can set up printing in seconds:

#### 1. Run the Install Script
Open terminal inside the cloned repository directory and execute the installation script as a regular user (it will automatically prompt for your administrator password when writing to system folders):

```bash
chmod +x install.sh uninstall.sh
./install.sh
```

*(To completely remove the native print driver at any point, simply run `./uninstall.sh` as a regular user).*

#### 2. Connect the USB Cable (Plug-and-Play)
Connect the printer to your Mac via USB. macOS's hardware auto-discovery daemon will match the printer's hardware ID with the registered PPD, and **automatically create the print queue** under **System Settings > Printers & Scanners**!

*(The printer will appear under the Model name `HP LaserJet Pro MFP m125a, hpcups 3.26.4 (str4ngemd ARM64)` and is ready to print natively!)*

> [!IMPORTANT]
> **Troubleshooting Auto-Registration Issues:**
> If the printer was connected *before* running `./install.sh`, macOS may have registered it with an incorrect/broken driver. To fix this:
> 1. **Unplug** the USB cable.
> 2. Go to **System Settings > Printers & Scanners**, right-click on the printer, and click **Remove Printer**.
> 3. **Plug the printer back in**. macOS will auto-discover it and build the queue using our native driver!

> I've included my username (str4ngemd) so it won't be confused for an official PPD. 
> 
> If you want to remove it, or write a custom name, \
>  open the `HP_LaserJet_Pro_MFP_M125a.ppd`, \
>  replace lines 12-14 to your desire, \
>  and re-run `./install.sh`.

---

### Option B: Manual Compilation from Source (Advanced / Custom Build)

If you prefer to audit and compile the driver yourself directly from the open-source HPLIP repository:

#### 1. Install Build Dependencies
Install Homebrew if not already installed, then fetch the compiler dependencies:
```bash
brew install jpeg cups
```

#### 2. Download and Extract HPLIP Source Code
Download version [`3.26.4`](https://sourceforge.net/projects/hplip/files/hplip/3.26.4/hplip-3.26.4.tar.gz/download) from [HPLIP SourceForge](https://sourceforge.net/projects/hplip/files/hplip/):
```bash
tar -xzf hplip-3.26.4.tar.gz
cd hplip-3.26.4
```

#### 3. Resolve Case-Insensitive Filename Collisions
Rename conflicting files in the HPLIP source codebase to prevent build failures on macOS's default case-insensitive filesystem:
```bash
mv prnt/hpcups/Utils.h prnt/hpcups/HPCupsUtils.h
mv prnt/hpcups/Utils.cpp prnt/hpcups/HPCupsUtils.cpp
```

#### 4. Apply Source Patches
*   **Header Rename**: Open `prnt/hpcups/HPCupsUtils.cpp` and update line 32 to include our renamed header:
    ```cpp
    #include "HPCupsUtils.h"
    #include "utils.h"
    ```
*   **Non-Portable Headers**: Open `prnt/hpcups/genJPEGStrips.cpp` and replace the Linux-specific headers (around line 31) with standard macOS headers:
    ```cpp
    // Replace #include <malloc.h> and <memory.h> with:
    #include <stdlib.h>
    #include <string.h>
    ```
*   **Encap Technology Stub**: Open `prnt/hpcups/EncapsulatorFactory.cpp` and replace its entire contents with the following minimal PCLm (`hbpl1`) selector to strip out unneeded targets:
    ```cpp
    #include "CommonDefinitions.h"
    #include "EncapsulatorFactory.h"
    #include "Encapsulator.h"
    #include "Hbpl1.h"
    #include <string.h>

    Encapsulator *EncapsulatorFactory::GetEncapsulator (char *encap_tech)
    {
        if (encap_tech == NULL) {
            return NULL;
        }
        if (!strcmp (encap_tech, "hbpl1"))
        {
            return new Hbpl1();
        }
        return NULL;
    }
    ```

#### 5. Build the Native Binary
Copy the `build_native.sh` script included in this repository to the root of the extracted HPLIP folder, make it executable, and run it:
```bash
cp ../build_native.sh ./
chmod +x build_native.sh
./build_native.sh
```
This links against Homebrew's static archive `/opt/homebrew/lib/libjpeg.a` and compiles a self-contained `hpcups-native` binary. You can now proceed with the installation steps in **Option A** to register the binary and PPD!

---

## Part 2: Native Scanning Setup (Apple ICA Driver)

We provide a **fully integrated native macOS Image Capture (ICA) Driver** for scanning. 

### How it Works
1. **`scan-go` Backend:** A compiled Go binary (`/Library/Printers/hpcups-str4ngemd/bin/scan-go`) that directly manages raw bulk USB transfers to send SOAP/XML commands, parse DIME data, and save JPEG scan outputs.
2. **`M125aScanner.app` Driver:** An Apple Image Capture (ICA) driver bundle installed at `/Library/Image Capture/Devices/M125aScanner.app`. It matches the M125a USB vendor/product IDs (`0x03F0`/`0x222A`) to auto-launch. When you scan from any native macOS app, this driver calls `scan-go` in the background and converts the outputs seamlessly.

### Installation
 
#### Option A: Quick Start (Turnkey Installation)
By default, the installer copies and registers the **pre-compiled native binaries** included in the repository. This requires **no dependencies** (no Go compiler, no Xcode, and no Xcode Command Line Tools) and works instantly:
```bash
chmod +x install_scan.sh uninstall_scan.sh
./install_scan.sh
```
*(To completely remove the scanner driver, run `./uninstall_scan.sh` as a regular user).*

#### Option B: Compile from Source (For Developers)
If you wish to modify the code or compile the binaries yourself from source, run the installer with the `build` argument:
```bash
./install_scan.sh build
```
*Note: Compiling from source requires Go (for `scan-go`) and Xcode (for the `M125aScanner` ICA app wrapper) to be installed.*

During installation, the script copies/compiles the binaries, signs the components locally using macOS's built-in ad-hoc codesigning (`codesign -s -`), clears quarantine attributes, and registers them inside `/Library`. Since macOS performs signing locally, this does not require internet access or developer certificates.

### Using the Scanner

1. Connect the printer/scanner to your Mac using the USB cable.
2. Open **Image Capture.app** (installed by default on macOS, find it in Spotlight).
3. The device **HP LaserJet Pro MFP M125a (str4ngemd)** will appear under the **Devices** section in the sidebar.
4. Select it, choose your resolution (75, 150, 300, 600, or 1200 DPI), select the output format (JPEG, PNG, PDF, TIFF), and click **Scan**!
5. The scan will run, and the output file will be saved directly to your chosen directory.

---

### Low-level Command Line Scan (Optional / Debugging)

If you wish to trigger a scan manually via the command line without the ICA GUI app:
```bash
/Library/Printers/hpcups-str4ngemd/bin/scan-go -r 300 -m Color -o output.jpg
```
Arguments:
* `-r`: Resolution in DPI (`75`, `150`, `300`, `600`, `1200`).
* `-m`: Color Mode (`Color`, `Gray`, `Mono`).
* `-o`: Output file path.
