package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MakeChunkedHTTPPost wraps the XML envelope in an HTTP chunked transfer format.
func MakeChunkedHTTPPost(xmlBody string) []byte {
	bodyBytes := []byte(xmlBody)
	chunkSizeHex := fmt.Sprintf("%X\r\n", len(bodyBytes))
	httpHeaders := "POST / HTTP/1.1\r\n" +
		"Host: http:0\r\n" +
		"User-Agent: gSOAP/2.7\r\n" +
		"Content-Type: application/soap+xml; charset=utf-8\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"Connection: close\r\n\r\n"

	result := append([]byte(httpHeaders), []byte(chunkSizeHex)...)
	result = append(result, bodyBytes...)
	result = append(result, []byte("\r\n0\r\n\r\n")...)
	return result
}

// IsChunkedResponseComplete returns true if the data contains a complete HTTP response
// terminated by the final chunk "0\r\n\r\n".
func IsChunkedResponseComplete(data []byte) bool {
	headerSeparator := []byte("\r\n\r\n")
	idx := bytes.Index(data, headerSeparator)
	if idx == -1 {
		return false
	}
	body := data[idx+4:]

	offset := 0
	for offset < len(body) {
		lineEnd := bytes.Index(body[offset:], []byte("\r\n"))
		if lineEnd == -1 {
			return false
		}
		chunkSizeStr := string(bytes.TrimSpace(body[offset : offset+lineEnd]))
		if chunkSizeStr == "" {
			offset += lineEnd + 2
			continue
		}
		chunkSize, err := strconv.ParseInt(chunkSizeStr, 16, 64)
		if err != nil {
			return false
		}
		if chunkSize == 0 {
			// Check if we have the final closing CRLF
			if len(body) >= offset+lineEnd+4 {
				return true
			}
			return false
		}
		offset += lineEnd + 2 + int(chunkSize) + 2
		if offset > len(body) {
			return false
		}
	}
	return false
}

// ReadResponse reads from the bulk IN endpoint until expectedTag is matched or timeout occurs.
func ReadResponse(dc *DeviceContext, expectedTag string, timeout time.Duration, isImage bool) ([]byte, error) {
	var respBytes []byte
	buf := make([]byte, 65536)
	var startTime time.Time

	readTimeout := timeout

	for {
		if startTime.IsZero() {
			startTime = time.Now()
		}

		n, err := dc.ReadBulk(buf, readTimeout)
		if err != nil {
			if time.Since(startTime) >= timeout {
				break
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if n == 0 {
			if time.Since(startTime) >= timeout {
				break
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		startTime = time.Now() // Reset idle timer
		respBytes = append(respBytes, buf[:n]...)

		if isImage {
			readTimeout = 15000 * time.Millisecond
		} else {
			readTimeout = 250 * time.Millisecond
		}

		if IsChunkedResponseComplete(respBytes) {
			decoded, err := DecodeHTTPChunked(respBytes)
			if err == nil && strings.Contains(string(decoded), expectedTag) {
				return respBytes, nil
			}
		}
	}

	decoded, _ := DecodeHTTPChunked(respBytes)
	if strings.Contains(string(decoded), expectedTag) {
		return respBytes, nil
	}

	return nil, fmt.Errorf("timeout waiting for response containing '%s'", expectedTag)
}

// SendRequest writes the SOAP envelope and awaits the matching response tag.
func SendRequest(dc *DeviceContext, xmlBody string, expectedTag string, timeout time.Duration) ([]byte, error) {
	payload := MakeChunkedHTTPPost(xmlBody)
	_, err := dc.WriteBulk(payload, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("write failed: %v", err)
	}
	return ReadResponse(dc, expectedTag, timeout, false)
}

// CancelJob cancels an active job on the scanner.
func CancelJob(dc *DeviceContext, jobID int) {
	log.Printf("Sending CancelJobRequest for Job ID %d...\n", jobID)
	xmlCancel := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope
	xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
	xmlns:SOAP-ENC="http://www.w3.org/2003/05/soap-encoding"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xmlns:xsd="http://www.w3.org/2001/XMLSchema"
	xmlns:wscn="http://tempuri.org/wscn.xsd">
	<SOAP-ENV:Body>
		<wscn:CancelJobRequest>
			<JobId>%d</JobId>
			<JobToken></JobToken>
			<DocumentDescription></DocumentDescription>
		</wscn:CancelJobRequest>
	</SOAP-ENV:Body>
</SOAP-ENV:Envelope>`, jobID)

	_, _ = SendRequest(dc, xmlCancel, "CancelRequestResponseType", 5*time.Second)
}

// CheckAndHealScanner checks the status and cancels any hanging jobs.
func CheckAndHealScanner(dc *DeviceContext) {
	log.Println("Checking scanner status and active jobs...")
	xmlGetElements := `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope
	xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
	xmlns:SOAP-ENC="http://www.w3.org/2003/05/soap-encoding"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xmlns:xsd="http://www.w3.org/2001/XMLSchema"
	xmlns:wscn="http://tempuri.org/wscn.xsd">
	<SOAP-ENV:Body>
		<wscn:GetScannerElements></wscn:GetScannerElements>
	</SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	resp, err := SendRequest(dc, xmlGetElements, "ScanElements", 10*time.Second)
	if err != nil {
		log.Printf("Warning: Could not fetch scanner status: %v\n", err)
		return
	}

	decoded, err := DecodeHTTPChunked(resp)
	if err != nil {
		return
	}
	bodyStr := string(decoded)

	stateRegex := regexp.MustCompile(`<ScannerState>(.*?)</ScannerState>`)
	stateMatch := stateRegex.FindStringSubmatch(bodyStr)
	state := "Unknown"
	if len(stateMatch) > 1 {
		state = stateMatch[1]
	}
	log.Printf("Current Scanner State: %s\n", state)

	activeJobsRegex := regexp.MustCompile(`(?s)<ActiveJobs>(.*?)</ActiveJobs>`)
	activeJobsMatch := activeJobsRegex.FindStringSubmatch(bodyStr)
	if len(activeJobsMatch) > 1 {
		jobIDRegex := regexp.MustCompile(`<JobId>(\d+)</JobId>`)
		jobIDs := jobIDRegex.FindAllStringSubmatch(activeJobsMatch[1], -1)
		if len(jobIDs) > 0 {
			log.Printf("Found active hanging jobs on scanner: %v. Healing device by cancelling them...\n", jobIDs)
			for _, match := range jobIDs {
				jid, err := strconv.Atoi(match[1])
				if err == nil {
					CancelJob(dc, jid)
				}
			}
			time.Sleep(1 * time.Second)
		}
	} else if state != "Idle" && state != "Unknown" {
		log.Printf("Scanner is in busy state '%s' but no active job IDs found in status.\n", state)
	}
}

func main() {
	resolution := flag.Int("r", 300, "Scan resolution in DPI (75, 150, 300, 600, 1200)")
	mode := flag.String("m", "Color", "Scan color mode (Color, Gray, Mono)")
	output := flag.String("o", "scan.jpg", "Output JPEG path")
	flag.Parse()

	// Clamp resolution to the closest supported native scanner resolution
	clampedRes := 300
	if *resolution < 100 {
		clampedRes = 75
	} else if *resolution < 200 {
		clampedRes = 150
	} else if *resolution < 450 {
		clampedRes = 300
	} else if *resolution < 900 {
		clampedRes = 600
	} else {
		clampedRes = 1200
	}
	*resolution = clampedRes

	// Map color mode string to scanner parameters
	colorMap := map[string]string{
		"Color": "RGB24",
		"Gray":  "GrayScale8",
		"Mono":  "BlackandWhite1",
	}
	colorProcessing, exists := colorMap[*mode]
	if !exists {
		log.Fatalf("Error: Invalid color mode '%s'. Use Color, Gray, or Mono.\n", *mode)
	}

	// Open scanner USB interface
	dc, err := OpenScanner()
	if err != nil {
		log.Fatalf("Error: %v\n", err)
	}
	defer dc.Close()

	// Drain endpoint
	dc.DrainInEndpoint()
	log.Println("Waiting 1.5s for printer to settle after draining...")
	time.Sleep(1500 * time.Millisecond)

	// Heal scanner state
	CheckAndHealScanner(dc)

	// 1. Create Scan Job Request
	log.Printf("Creating scan job (%d DPI, %s)...\n", *resolution, *mode)
	xmlCreateJob := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope
	xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
	xmlns:SOAP-ENC="http://www.w3.org/2003/05/soap-encoding"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xmlns:xsd="http://www.w3.org/2001/XMLSchema"
	xmlns:wscn="http://tempuri.org/wscn.xsd">
	<SOAP-ENV:Body>
		<wscn:CreateScanJobRequest>
			<ScanIdentifier></ScanIdentifier>
			<ScanTicket>
				<JobDescription></JobDescription>
				<DocumentParameters>
					<Format>jfif</Format>
					<CompressionQualityFactor>0</CompressionQualityFactor>
					<ImagesToTransfer>0</ImagesToTransfer>
					<InputSource>Platen</InputSource>
					<ContentType>Auto</ContentType>
					<InputSize>
						<InputMediaSize>
							<Width>8499</Width>
							<Height>11689</Height>
						</InputMediaSize>
						<DocumentSizeAutoDetect>false</DocumentSizeAutoDetect>
					</InputSize>
					<Exposure>
						<AutoExposure>false</AutoExposure>
						<ExposureSettings>
							<Contrast>0</Contrast>
							<Brightness>0</Brightness>
						</ExposureSettings>
					</Exposure>
					<MediaSides>
						<MediaFront>
							<ScanRegion>
								<ScanRegionXOffset>0</ScanRegionXOffset>
								<ScanRegionYOffset>0</ScanRegionYOffset>
								<ScanRegionWidth>8499</ScanRegionWidth>
								<ScanRegionHeight>11689</ScanRegionHeight>
							</ScanRegion>
							<ColorProcessing>%s</ColorProcessing>
							<Resolution>
								<Width>%d</Width>
								<Height>%d</Height>
							</Resolution>
						</MediaFront>
					</MediaSides>
				</DocumentParameters>
				<RetrieveImageTimeout>300</RetrieveImageTimeout>
				<ScanManufacturingParameters>
					<DisableImageProcessing>false</DisableImageProcessing>
				</ScanManufacturingParameters>
			</ScanTicket>
		</wscn:CreateScanJobRequest>
	</SOAP-ENV:Body>
</SOAP-ENV:Envelope>`, colorProcessing, *resolution, *resolution)

	resp, err := SendRequest(dc, xmlCreateJob, "CreateScanJobResponseType", 60*time.Second)
	if err != nil {
		log.Fatalf("Error: CreateScanJobRequest failed: %v\n", err)
	}

	decoded, err := DecodeHTTPChunked(resp)
	if err != nil {
		log.Fatalf("Error: Failed to decode job creation HTTP payload: %v\n", err)
	}

	jobMatchRegex := regexp.MustCompile(`<JobId>(\d+)</JobId>`)
	jobMatch := jobMatchRegex.FindStringSubmatch(string(decoded))
	if len(jobMatch) < 2 {
		log.Fatalf("Error: Failed to parse JobId from XML response: %s\n", string(decoded))
	}
	jobID, err := strconv.Atoi(jobMatch[1])
	if err != nil {
		log.Fatalf("Error: Invalid JobId parsed: %s\n", jobMatch[1])
	}
	log.Printf("Scan Job created with ID: %d\n", jobID)

	// Wait for optical carriage warmup
	log.Println("Waiting 2.0s for scanner head to initialize job...")
	time.Sleep(2 * time.Second)

	// 2. Retrieve Image
	log.Println("Starting scan and retrieving image stream...")
	xmlRetrieve := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope
	xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
	xmlns:SOAP-ENC="http://www.w3.org/2003/05/soap-encoding"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xmlns:xsd="http://www.w3.org/2001/XMLSchema"
	xmlns:wscn="http://tempuri.org/wscn.xsd">
	<SOAP-ENV:Body>
		<wscn:RetrieveImageRequest>
			<JobId>%d</JobId>
			<JobToken></JobToken>
			<DocumentDescription></DocumentDescription>
		</wscn:RetrieveImageRequest>
	</SOAP-ENV:Body>
</SOAP-ENV:Envelope>`, jobID)

	payload := MakeChunkedHTTPPost(xmlRetrieve)
	_, err = dc.WriteBulk(payload, 5*time.Second)
	if err != nil {
		log.Fatalf("Error: RetrieveImage request failed: %v\n", err)
	}

	rawImageData, err := ReadResponse(dc, "RetrieveImageRequestResponse", 60*time.Second, true)
	if err != nil {
		log.Fatalf("Error: Image transfer failed: %v\n", err)
	}
	log.Printf("Transfer finished. Received %d raw bytes.\n", len(rawImageData))

	// 3. Parse DIME container
	log.Println("Parsing DIME image container...")
	decodedImageBody, err := DecodeHTTPChunked(rawImageData)
	if err != nil {
		log.Fatalf("Error: Failed to decode image body HTTP chunks: %v\n", err)
	}

	records, err := ParseDime(decodedImageBody)
	if err != nil {
		log.Fatalf("Error: Failed to parse DIME container: %v\n", err)
	}

	// Reconstruct JPEG bytes
	var jpegData []byte
	foundImage := false
	for _, rec := range records {
		isImageRecord := strings.Contains(rec.Type, "image") || strings.Contains(rec.Type, "jfif") || rec.Type == "image/jfif"
		if isImageRecord {
			foundImage = true
			jpegData = append(jpegData, rec.Data...)
		} else if foundImage && rec.Type == "" && rec.ID == "" {
			// Append chunked data continuation record
			jpegData = append(jpegData, rec.Data...)
		} else if foundImage {
			break
		}
	}

	if !foundImage || len(jpegData) == 0 {
		log.Fatalf("Error: Could not find image payload in the DIME scan response.\n")
	}

	// Save output JPEG file
	err = os.WriteFile(*output, jpegData, 0644)
	if err != nil {
		log.Fatalf("Error: Failed to save scan output to '%s': %v\n", *output, err)
	}

	log.Printf("Success! Scanned image saved to: %s (size: %d bytes)\n", *output, len(jpegData))

	// Cleanup job on exit
	log.Println("Cleaning up job on scanner...")
	CancelJob(dc, jobID)
	log.Println("Scanner released.")
}
