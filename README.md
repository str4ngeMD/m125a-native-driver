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

## Part 2: Native Scanning Setup (Userspace Driver)

> This is a working Proof of Concept.
>
> You **can** scan with the python script. \
> But Image Capture does not recognize as a scanner now.

The scanner uses the standard Web Services on Devices (WSD) scanning protocol. Since the official scanning backend depends on a closed-source Linux ELF library (`bb_soap.so`), we bypass it completely using a userspace Python script that reads the scanner over raw bulk USB endpoints.

### Step-by-Step Setup

#### 1. Set Up Python Environment
Create a virtual environment and install PyUSB to handle USB bus access:
```bash
python3 -m venv venv
./venv/bin/pip install pyusb
```

#### 2. Perform Scans
Execute `scan.py` to trigger optical scanning, passing in the resolution (`-r`) and output file name (`-o`):

*   **75 DPI Low Res Scan**:
    ```bash
    ./venv/bin/python3 scan.py -r 75 -o "page_75dpi.jpg"
    ```
*   **300 DPI Standard Scan**:
    ```bash
    ./venv/bin/python3 scan.py -r 300 -o "page_300dpi.jpg"
    ```
*   **600 DPI High Res Scan**:
    ```bash
    ./venv/bin/python3 scan.py -r 600 -o "page_600dpi.jpg"
    ```
*   **1200 DPI Optical Maximum**:
    ```bash
    ./venv/bin/python3 scan.py -r 1200 -o "page_1200dpi.jpg"
    ```
