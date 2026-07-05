# Project TODO List

This document lists future improvements and optimization ideas for the native macOS driver of the HP LaserJet Pro MFP M125a.

## 🚀 Future Scanning Enhancements

### 1. Hardware-Level Partial Scans (Crop Region Pass-Through)
*   **Current Behavior:** When a custom selection box/crop area is set in Image Capture, the scanner initiates a full-page scan. The `M125aScanner` driver then crops the resulting full-size image in software before presenting it to the OS.
*   **Target Improvement:** Upgrade the `scan-go` backend and `M125aScanner` frontend to accept and pass scan coordinates down to the hardware.
    *   **Go Backend:** Add support for flags like `-x`, `-y`, `-w`, and `-h` (or `-width`, `-height`) to specify scan boundaries.
    *   **SOAP/XML Payload:** Reverse engineer the XML payload configuration in the SOAP requests to discover where region bounds/offsets are sent to the printer.
    *   **Objective-C Wrapper:** Extract the selection box coordinates inside `setSelectedFunctionalUnitParams:` and pass them as arguments to `scan-go` during launch.
*   **Benefit:** Reduces scanner head travel distance and scan time for smaller selections, saving physical wear-and-tear and speeding up operations.
