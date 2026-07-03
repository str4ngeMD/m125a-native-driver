# Research History & Technical Design Notes (MFP M125a)

This document preserves the context, experimental setups, protocol analysis, and engineering workarounds implemented to achieve native printing and scanning on macOS Apple Silicon for the HP LaserJet Pro MFP M125a.

---

## 1. Background & Protocol Analysis

The HP LaserJet Pro MFP M125a is a USB composite device. On the printing side, it expects rasterized page data formatted in PCLm (`hbpl1`). On the scanning side, it implements the **WS-Scan (Web Services Scan)** protocol over raw USB bulk endpoints.

To understand the communication protocol, we captured and reverse-engineered the raw USB frames when performing scans inside a working environment.

### Experimental Setup
1.  **Virtualization**: We ran an ARM64 Ubuntu Linux VM inside UTM (using QEMU) and passed the physical MFP M125a USB device through to the guest VM.
2.  **HPLIP Installation**: We installed the standard HPLIP suite and ran the interactive installer to fetch the proprietary scanner plugin:
    ```bash
    sudo apt update
    sudo apt install hplip hplip-gui sane-utils
    hp-plugin -i   # Downloads the proprietary plugin containing bb_soap.so
    hp-setup -i    # Registers the device
    ```
3.  **USB Bus Identification**: We ran `lsusb` to check the USB bus layout inside the Linux guest:
    ```bash
    $ lsusb
    Bus 003 Device 002: ID 03f0:222a HP, Inc LaserJet Pro MFP M125nw
    ```
    This mapped the scanner device to **Bus 3** at **Device Address 2**.
4.  **USB Packet Sniffing**: We loaded the kernel's `usbmon` module and ran Wireshark as root inside the VM, selecting the `usbmon3` interface (matching Bus 003) and filtering traffic by `usb.device_address == 2`:
    ```bash
    sudo modprobe usbmon
    sudo wireshark
    ```
5.  **Data Extraction**:
    After capturing a 75 DPI scan, we exported the raw USB bulk transmission packets into a payload hex dump file using `tshark`:
    ```bash
    tshark -r scan_low.pcapng -Y "usb.device_address == 2 && usb.capdata" -T fields -e usb.capdata > payloads_lowq_hex.txt
    ```
6.  **Hex Decoding**:
    To translate the hex stream into readable ASCII (SOAP XML) and binary files, we used a Perl one-liner (which bypasses the "File too big" limits of standard `xxd -r` translations):
    ```bash
    perl -ne 's/([0-9a-fA-F]{2})/print pack("H2", $1)/eg' payloads_lowq_hex.txt > ascii_lowq.txt
    ```
    By deleting the large binary JPEG chunks from `ascii_lowq.txt`, we produced `ascii_lowq_readable.txt` to inspect the clean SOAP protocol structure.

---

## 2. Scanning Protocol Discoveries

We discovered that although the scanner communicates over raw USB bulk endpoints (Interface `0`, `EP_OUT 0x02` and `EP_IN 0x82`), it acts as a web service. It wraps all commands in **SOAP 1.2 XML envelopes** and sends them over an **HTTP/1.1 Chunked Transfer** layer.

### The XML SOAP Transactions
The communication follows a strict sequence:
1.  **GetScannerElements**: The host sends a request to query the scanner capabilities (resolutions, supported color modes).
2.  **CreateScanJobRequest**: The host sends the desired scan settings (e.g., Mode: `Color`, Resolution: `300`, Format: `pdf/jfif`). The scanner returns a `<JobId>` (e.g. `1` or `2`).
3.  **RetrieveImageRequest**: The host requests the image stream for the active job ID. The scanner starts the motor and streams the binary image data.
4.  **CancelJobRequest**: The host cleans up and frees the scanner state.

### Reconstructing the Image (DIME Framing)
The binary data returned from `RetrieveImageRequest` is not a raw JPEG. It is wrapped in **DIME (Direct Internet Message Encapsulation)** multipart records. 
* A single scan page is split into multiple DIME records.
* The first record contains a SOAP envelope indicating the stream parameters.
* Subsequent records contain chunks of the raw JPEG (JFIF) image data.
* Some records are "continuation records" that lack standard HTTP headers. The script (`scan.py`) parses the DIME header lengths, extracts the binary segments, and concatenates them to rebuild the final `.jpg` file.

---

## 3. Resolving macOS USB Engine Obstacles

When writing the userspace Python driver (`scan.py`) for macOS, we solved three major OS-level USB communication challenges:

### A. Endpoint Stall Recovery (`clear_halt`)
*   **The Issue**: Under macOS's USB stack, if a bulk read times out (e.g. while waiting for the scanner head to warm up), the OS automatically places the bulk `EP_IN` endpoint in a **Halted/Stalled** state. Any subsequent read requests instantly return `[Errno 32] Pipe error` instead of waiting.
*   **The Solution**: We wrapped all read calls in exception handlers. When a timeout occurs, the script explicitly calls:
    ```python
    dev.clear_halt(EP_IN)
    ```
    This resets the host controller's stall bit, keeping the pipe alive.

### B. Warm-Up & Calibration Delays
*   **The Issue**: High-resolution scans (600/1200 DPI) require the scanning head to perform an optical calibration and warm-up sequence before it starts generating pixels. This sequence takes up to 20-30 seconds, which exceeds standard USB timeout thresholds.
*   **The Solution**: We configured the `CreateScanJobRequest` timeout to 60 seconds (`timeout=60000`) and implemented active idle reset loops to give the carriage array ample time to calibrate.

### C. ZLP (Zero Length Packet) Polling
*   **The Issue**: The scanner's USB interface frequently returns 0-byte frames (Zero Length Packets) to signal "no data ready yet" rather than NAK'ing. Standard libraries treat a 0-length read as EOF.
*   **The Solution**: The script distinguishes ZLPs from a true EOF by checking a global timeout clock. If it reads a ZLP, it sleeps for 100ms and tries again until the timeout expires.

---

## 4. Printing Solution (Sandbox & Collision Resolution)

Compiling the open-source HPLIP `hpcups` print filter on macOS required resolving two unique system barriers:

### A. Case-Insensitive Filename Clash
HPLIP was developed on case-sensitive Linux filesystems. In the HPLIP source, `prnt/hpcups/Utils.h` (the filter utilities) and `common/utils.h` (the database utilities) collide on macOS's default case-insensitive filesystems, causing headers to overwrite each other during decompression.
* We resolved this by renaming `Utils.h`/`Utils.cpp` to `HPCupsUtils.h`/`HPCupsUtils.cpp` and patching the local `#include` references.

### B. CUPS Sandboxing Bypass: Dynamic vs. Static Linking
Under macOS, the CUPS daemon runs filters under the restricted `_lp` user inside a strict Apple Sandbox. This sandbox completely blocks access to Homebrew installations (`/opt/homebrew`), causing the filter to crash with a dynamic linker error if it links dynamically to Brew's `libjpeg.8.dylib`.

We developed two ways to resolve this sandbox restriction:

1. **Loader Path Patching (Dylib)**:
   We can copy the dynamic library (`libjpeg.8.dylib`) into the print filter directory `/Library/Printers/hpcups/filter/lib/` and run `install_name_tool -change` to modify the binary's load commands to reference it relatively via `@loader_path/lib/libjpeg.8.dylib` (where `@loader_path` is a dynamic runtime macro resolving to the directory containing the loading executable). This forces the program to load the library locally without querying the sandboxed Homebrew path.
2. **Static Linking (The Cleanest Solution)**:
   Instead of copying and patching dynamic libraries, we link directly against Homebrew's static archive archive (**`libjpeg.a`**) during the build process:
   ```bash
   clang++ *.o /opt/homebrew/lib/libjpeg.a -lcups -lz -o hpcups-native
   ```
   By supplying the `.a` archive path directly to the linker, the compiler copies the required machine code of `libjpeg` directly *into* the final `hpcups-native` binary. The executable is completely self-contained, requires no library dependency copying, no `install_name_tool` patching, and bypasses the sandbox completely!

