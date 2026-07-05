package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/gousb"
)

const (
	VendorID    = 0x03F0
	ProductID   = 0x222A
	InterfaceNum = 0
	EndpointOut = 0x02
	EndpointIn  = 0x82
)

// DeviceContext holds references to the opened USB device and active endpoints.
type DeviceContext struct {
	ctx      *gousb.Context
	dev      *gousb.Device
	doneFunc func()
	intf     *gousb.Interface
	inEP     *gousb.InEndpoint
	outEP    *gousb.OutEndpoint
}

// OpenScanner searches for the M125a scanner on the USB bus, claims interface 0,
// and sets up the active bulk endpoints.
func OpenScanner() (*DeviceContext, error) {
	ctx := gousb.NewContext()

	// Find matching USB device
	devs, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == gousb.ID(VendorID) && desc.Product == gousb.ID(ProductID)
	})
	if err != nil {
		ctx.Close()
		return nil, fmt.Errorf("error querying USB devices: %v", err)
	}
	if len(devs) == 0 {
		ctx.Close()
		return nil, fmt.Errorf("HP LaserJet Pro MFP M125 scanner not found on USB bus")
	}

	// Use the first matching device and close others if any
	dev := devs[0]
	for i := 1; i < len(devs); i++ {
		devs[i].Close()
	}

	// Disable auto detach on macOS as detaching kernel drivers is not supported/needed and causes fatal bad access errors.
	_ = dev.SetAutoDetach(false)

	// Get active configuration number to avoid libusb_set_configuration access errors on macOS
	cfgNum, err := dev.ActiveConfigNum()
	if err != nil {
		cfgNum = 1
	}

	cfg, err := dev.Config(cfgNum)
	if err != nil {
		dev.Close()
		ctx.Close()
		return nil, fmt.Errorf("failed to open configuration %d: %v", cfgNum, err)
	}

	intf, err := cfg.Interface(InterfaceNum, 0)
	if err != nil {
		cfg.Close()
		dev.Close()
		ctx.Close()
		return nil, fmt.Errorf("failed to claim interface %d: %v", InterfaceNum, err)
	}

	doneFunc := func() {
		intf.Close()
		cfg.Close()
	}

	// Open the bulk IN endpoint
	inEP, err := intf.InEndpoint(EndpointIn)
	if err != nil {
		doneFunc()
		dev.Close()
		ctx.Close()
		return nil, fmt.Errorf("failed to open IN endpoint 0x%02x: %v", EndpointIn, err)
	}

	// Open the bulk OUT endpoint
	outEP, err := intf.OutEndpoint(EndpointOut)
	if err != nil {
		doneFunc()
		dev.Close()
		ctx.Close()
		return nil, fmt.Errorf("failed to open OUT endpoint 0x%02x: %v", EndpointOut, err)
	}

	return &DeviceContext{
		ctx:      ctx,
		dev:      dev,
		doneFunc: doneFunc,
		intf:     intf,
		inEP:     inEP,
		outEP:    outEP,
	}, nil
}

// Close releases the claimed interface and closes the USB device context.
func (dc *DeviceContext) Close() {
	if dc.doneFunc != nil {
		dc.doneFunc()
	}
	if dc.dev != nil {
		dc.dev.Close()
	}
	if dc.ctx != nil {
		dc.ctx.Close()
	}
}

// ClearHalt clears a stalled state on a specific endpoint by issuing a USB control transfer.
func (dc *DeviceContext) ClearHalt(endpoint uint8) error {
	// Standard USB Control Request:
	//bmRequestType = 0x02 (Recipient: Endpoint)
	//bRequest      = 0x01 (CLEAR_FEATURE)
	//wValue         = 0x0000 (ENDPOINT_HALT)
	//wIndex         = endpoint address (0x82 or 0x02)
	_, err := dc.dev.Control(0x02, 0x01, 0x0000, uint16(endpoint), nil)
	return err
}

// WriteBulk sends data to the scanner's bulk OUT endpoint with a timeout.
func (dc *DeviceContext) WriteBulk(data []byte, timeout time.Duration) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	n, err := dc.outEP.WriteContext(ctx, data)
	return n, err
}

// ReadBulk reads data from the scanner's bulk IN endpoint with a timeout.
func (dc *DeviceContext) ReadBulk(buf []byte, timeout time.Duration) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	n, err := dc.inEP.ReadContext(ctx, buf)
	return n, err
}

// DrainInEndpoint consumes any stale data remaining on the IN endpoint.
func (dc *DeviceContext) DrainInEndpoint() {
	log.Println("Draining stale bytes from IN endpoint...")
	drainedBytes := 0
	buf := make([]byte, 65536)

	// First read timeout is longer to catch any queued data
	timeout := 1000 * time.Millisecond

	for {
		n, err := dc.ReadBulk(buf, timeout)
		if err != nil {
			// Clear halt in case of timeout/error
			_ = dc.ClearHalt(EndpointIn)
			break
		}
		if n == 0 {
			break
		}
		drainedBytes += n
		// Shorter timeout for subsequent reads to check if pipeline is empty
		timeout = 100 * time.Millisecond
	}

	if drainedBytes > 0 {
		log.Printf("Drained %d stale bytes.\n", drainedBytes)
	} else {
		log.Println("IN endpoint was clean.")
	}
}
