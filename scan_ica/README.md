This project is a native macOS Image Capture (ICA) driver shim/adapter (ported from [virtual-scanner](https://github.com/yushulx/virtual-scanner/tree/main/macos)). It acts as a systems-level frontend for the `scan_go` USB backend, translating macOS ICA framework requests into local subprocess executions of `scan-go` to drive the physical hardware.

![Image Capture natively displays our printer!](image_capture_screenshot.png)

---

# macOS Virtual Scanner
This project is ported from Apple's [Virtual Scanner project](https://developer.apple.com/library/archive/samplecode/VirtualScanner/Introduction/Intro.html).

https://github.com/user-attachments/assets/87718501-f96a-49cb-b466-8ffe2200b76e

## How to Use
1. Import the project into Xcode.
2. Build and run the project.
3. Test the virtual scanner with `Image Capture` or [Dynamic Web TWAIN online demo](https://demo3.dynamsoft.com/web-twain/).

    ![macOS virtual scanner](https://www.dynamsoft.com/codepool/img/2025/03/macos-virtual-scanner.png)
