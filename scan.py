import usb.core
import usb.util
import re
import sys
import argparse
import time

VENDOR_ID = 0x03F0
PRODUCT_ID = 0x222A

# SOAP Endpoint Configuration
INTERFACE_NUM = 0
EP_OUT = 0x02
EP_IN = 0x82

def make_chunked_http_post(xml_body):
    body_bytes = xml_body.encode('utf-8')
    chunk_size_hex = f"{len(body_bytes):X}\r\n"
    
    http_headers = (
        "POST / HTTP/1.1\r\n"
        "Host: http:0\r\n"
        "User-Agent: gSOAP/2.7\r\n"
        "Content-Type: application/soap+xml; charset=utf-8\r\n"
        "Transfer-Encoding: chunked\r\n"
        "Connection: close\r\n\r\n"
    )
    return http_headers.encode('utf-8') + chunk_size_hex.encode('utf-8') + body_bytes + b"\r\n0\r\n\r\n"

def drain_in_endpoint(dev):
    print("Draining stale bytes from IN endpoint...")
    drained_bytes = 0
    try:
        # First read has a longer timeout (1000ms) to allow data to arrive
        timeout = 1000
        while True:
            chunk = dev.read(EP_IN, 65536, timeout=timeout)
            if not chunk or len(chunk) == 0:
                break
            drained_bytes += len(chunk)
            timeout = 100 # Short timeout for subsequent reads
    except usb.core.USBError:
        try:
            dev.clear_halt(EP_IN)
        except Exception:
            pass
    if drained_bytes > 0:
        print(f"Drained {drained_bytes} stale bytes.")
    else:
        print("IN endpoint was clean.")

def is_chunked_response_complete(data):
    if b"\r\n\r\n" not in data:
        return False
    parts = data.split(b"\r\n\r\n", 1)
    body = parts[1]
    
    idx = 0
    while idx < len(body):
        line_end = body.find(b"\r\n", idx)
        if line_end == -1:
            return False
        chunk_size_str = body[idx:line_end].strip()
        if not chunk_size_str:
            idx = line_end + 2
            continue
        try:
            chunk_size = int(chunk_size_str, 16)
        except ValueError:
            return False
        if chunk_size == 0:
            if len(body) >= line_end + 4:
                return True
            return False
        
        idx = line_end + 2 + chunk_size + 2
        if idx > len(body):
            return False
    return False

def get_xml_body(response_bytes):
    if b"\r\n\r\n" in response_bytes:
        parts = response_bytes.split(b"\r\n\r\n", 1)
        body = parts[1]
    else:
        body = response_bytes
        
    print(f"DEBUG get_xml_body: body len={len(body)}, start={repr(body[:20])}")
    unchunked = b""
    idx = 0
    while idx < len(body):
        line_end = body.find(b"\r\n", idx)
        if line_end == -1:
            print("DEBUG get_xml_body: line_end not found")
            break
        chunk_size_str = body[idx:line_end].strip()
        if not chunk_size_str:
            idx = line_end + 2
            continue
        try:
            chunk_size = int(chunk_size_str, 16)
        except ValueError:
            print(f"DEBUG get_xml_body: ValueError parsing hex '{chunk_size_str}'")
            break
        if chunk_size == 0:
            print("DEBUG get_xml_body: chunk_size is 0, normal break")
            break
        idx = line_end + 2
        chunk_data = body[idx:idx+chunk_size]
        print(f"DEBUG get_xml_body: Read chunk of size {chunk_size} (actually got {len(chunk_data)} bytes)")
        unchunked += chunk_data
        idx += chunk_size + 2
    return unchunked

def read_latest_response(dev, expected_tag, timeout=10000, is_image=False):
    print(f"DEBUG: Waiting for the latest response containing '{expected_tag}' (timeout={timeout}ms, is_image={is_image})...")
    latest_resp = b""
    resp_bytes = b""
    start_time = time.time()
    
    try:
        while True:
            print(f"DEBUG read_latest_response: Calling dev.read with timeout={timeout}ms...")
            chunk = dev.read(EP_IN, 65536, timeout=timeout)
            print(f"DEBUG read_latest_response: dev.read returned {len(chunk) if chunk else 0} bytes.")
            
            if not chunk or len(chunk) == 0:
                elapsed = (time.time() - start_time) * 1000.0
                if elapsed >= timeout:
                    print("DEBUG read_latest_response: Timeout exceeded on 0-byte reads.")
                    break
                time.sleep(0.1)
                continue
                
            start_time = time.time() # Reset start_time to implement proper idle timeout
            resp_bytes += chunk.tobytes()
            
            # Change timeout for subsequent chunks
            if is_image:
                timeout = 15000 # 15s timeout for slow physical scanning chunks
            else:
                timeout = 250 # 250ms timeout for fast XML responses
            
            while is_chunked_response_complete(resp_bytes):
                # Isolate the first complete HTTP response
                parts = resp_bytes.split(b"\r\n\r\n", 1)
                header = parts[0]
                body = parts[1]
                
                # Find the end of this chunked body
                idx = 0
                while idx < len(body):
                    line_end = body.find(b"\r\n", idx)
                    if line_end == -1:
                        break
                    chunk_size_str = body[idx:line_end].strip()
                    if not chunk_size_str:
                        idx = line_end + 2
                        continue
                    try:
                        chunk_size = int(chunk_size_str, 16)
                    except ValueError:
                        break
                    if chunk_size == 0:
                        idx = line_end + 2
                        idx += 2 # Skip the final \r\n after 0\r\n
                        break
                    idx = line_end + 2 + chunk_size + 2
                
                complete_resp_len = len(header) + 4 + idx
                complete_resp = resp_bytes[:complete_resp_len]
                
                # Slice off from resp_bytes
                resp_bytes = resp_bytes[complete_resp_len:]
                
                # Parse
                xml_body = get_xml_body(complete_resp)
                xml_body_str = xml_body.decode('utf-8', errors='replace')
                
                if expected_tag in xml_body_str:
                    latest_resp = complete_resp
                    print(f"DEBUG: Found matching response of {len(complete_resp)} bytes.")
                else:
                    print(f"DEBUG: Discarding non-matching response of {len(complete_resp)} bytes (tag '{expected_tag}' not found). Full body:\n{xml_body_str}\n")
                    
    except usb.core.USBError as e:
        print(f"DEBUG read_latest_response: USBError caught (clearing halt): {e}")
        try:
            dev.clear_halt(EP_IN)
        except Exception:
            pass
        
    if latest_resp:
        print(f"DEBUG: Returning latest matching response of {len(latest_resp)} bytes.")
    else:
        print(f"DEBUG: No response containing '{expected_tag}' was found.")
    return latest_resp

def send_request(dev, xml_body, expected_tag, timeout=10000):
    payload = make_chunked_http_post(xml_body)
    print(f"DEBUG send_request: Writing {len(payload)} bytes to EP_OUT...")
    try:
        bytes_written = dev.write(EP_OUT, payload, timeout=5000)
        print(f"DEBUG send_request: Successfully wrote {bytes_written} bytes.")
    except usb.core.USBError as e:
        print(f"DEBUG send_request: Write failed: {e}")
        return b""
    return read_latest_response(dev, expected_tag, timeout=timeout)

def parse_dime(data):
    unchunked = get_xml_body(data)
    idx = 0
    records = []
    current_chunked_payload = None
    
    while idx < len(unchunked):
        if idx + 12 > len(unchunked):
            break
        header = unchunked[idx:idx+12]
        
        # Byte 0 has flags: MB (0x80), ME (0x40), CF (0x20)
        flags = header[0]
        cf = bool(flags & 0x20)
        
        opt_len = int.from_bytes(header[2:4], 'big')
        id_len = int.from_bytes(header[4:6], 'big')
        type_len = int.from_bytes(header[6:8], 'big')
        data_len = int.from_bytes(header[8:12], 'big')
        
        opt_padded = (opt_len + 3) & ~3
        id_padded = (id_len + 3) & ~3
        type_padded = (type_len + 3) & ~3
        data_padded = (data_len + 3) & ~3
        
        idx += 12
        
        opt_data = unchunked[idx:idx+opt_len]
        idx += opt_padded
        
        id_data = unchunked[idx:idx+id_len]
        idx += id_padded
        
        type_data = unchunked[idx:idx+type_len]
        idx += type_padded
        
        data_data = unchunked[idx:idx+data_len]
        idx += data_padded
        
        if current_chunked_payload is not None:
            # Append this chunk's data to the active payload
            current_chunked_payload['data'] += data_data
            if not cf:
                # This was the final chunk, so complete and save the record
                records.append({
                    'id': current_chunked_payload['id'],
                    'type': current_chunked_payload['type'],
                    'data': bytes(current_chunked_payload['data'])
                })
                current_chunked_payload = None
        else:
            if cf:
                # Start of a chunked payload sequence
                current_chunked_payload = {
                    'id': id_data,
                    'type': type_data,
                    'data': bytearray(data_data)
                }
            else:
                # A single unchunked record
                records.append({
                    'id': id_data,
                    'type': type_data,
                    'data': data_data
                })
                
    return records

def cancel_job(dev, job_id):
    print(f"Sending CancelJobRequest for Job ID {job_id}...")
    xml_cancel = f'''<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope
	xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
	xmlns:SOAP-ENC="http://www.w3.org/2003/05/soap-encoding"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xmlns:xsd="http://www.w3.org/2001/XMLSchema"
	xmlns:wscn="http://tempuri.org/wscn.xsd">
	<SOAP-ENV:Body>
		<wscn:CancelJobRequest>
			<JobId>{job_id}</JobId>
			<JobToken></JobToken>
			<DocumentDescription></DocumentDescription>
		</wscn:CancelJobRequest>
	</SOAP-ENV:Body>
</SOAP-ENV:Envelope>'''
    try:
        send_request(dev, xml_cancel, "CancelRequestResponseType")
    except Exception as e:
        print(f"Warning: Cancel request failed: {e}")

def check_and_heal_scanner(dev):
    print("Checking scanner status and active jobs...")
    xml_get_elements = (
        '<?xml version="1.0" encoding="UTF-8"?>\n'
        '<SOAP-ENV:Envelope\n'
        '\txmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"\n'
        '\txmlns:SOAP-ENC="http://www.w3.org/2003/05/soap-encoding"\n'
        '\txmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"\n'
        '\txmlns:xsd="http://www.w3.org/2001/XMLSchema"\n'
        '\txmlns:wscn="http://tempuri.org/wscn.xsd">\n'
        '\t<SOAP-ENV:Body>\n'
        '\t\t<wscn:GetScannerElements></wscn:GetScannerElements>\n'
        '\t</SOAP-ENV:Body>\n'
        '</SOAP-ENV:Envelope>'
    )
    resp = send_request(dev, xml_get_elements, "ScanElements")
    if not resp:
        print("Warning: Could not fetch scanner status.")
        return
        
    xml_body = get_xml_body(resp).decode('utf-8', errors='replace')
    
    state_match = re.search(r'<ScannerState>(.*?)</ScannerState>', xml_body)
    state = state_match.group(1) if state_match else "Unknown"
    print(f"Current Scanner State: {state}")
    
    active_jobs_match = re.search(r'<ActiveJobs>(.*?)</ActiveJobs>', xml_body, re.DOTALL)
    if active_jobs_match:
        active_jobs_xml = active_jobs_match.group(1)
        job_ids = re.findall(r'<JobId>(\d+)</JobId>', active_jobs_xml)
        if job_ids:
            print(f"Found active hanging jobs on scanner: {job_ids}. Healing device by cancelling them...")
            for jid in job_ids:
                cancel_job(dev, int(jid))
            time.sleep(1.0)
    elif state != "Idle" and state != "Unknown":
        print(f"Scanner is in busy state '{state}' but no active job IDs found in status.")

def main():
    parser = argparse.ArgumentParser(description="HP LaserJet Pro MFP M125a Native USB Scanner CLI")
    parser.add_argument("-r", "--resolution", type=int, choices=[75, 150, 300, 600, 1200], default=300, help="Scan resolution in DPI (default: 300)")
    parser.add_argument("-m", "--mode", choices=["Color", "Gray", "Mono"], default="Color", help="Scan color mode (default: Color)")
    parser.add_argument("-o", "--output", default="scan.jpg", help="Output JPEG path (default: scan.jpg)")
    args = parser.parse_args()

    # Map modes
    mode_map = {
        "Color": "RGB24",
        "Gray": "GrayScale8",
        "Mono": "BlackandWhite1"
    }
    color_processing = mode_map[args.mode]

    # Find USB scanner
    dev = usb.core.find(idVendor=VENDOR_ID, idProduct=PRODUCT_ID)
    if dev is None:
        print("Error: HP LaserJet Pro MFP M125 scanner not found on USB.")
        sys.exit(1)

    # Detach kernel if occupied
    try:
        if dev.is_kernel_driver_active(INTERFACE_NUM):
            dev.detach_kernel_driver(INTERFACE_NUM)
    except Exception:
        pass

    usb.util.claim_interface(dev, INTERFACE_NUM)
    print("Claimed scanner interface.")
    drain_in_endpoint(dev)
    print("Waiting 1.5s for printer to settle after draining...")
    time.sleep(1.5)
    check_and_heal_scanner(dev)
    
    job_id = None
    try:
        # 1. Create Scan Job Request
        print(f"Creating scan job ({args.resolution} DPI, {args.mode})...")
        xml_create_job = f'''<?xml version="1.0" encoding="UTF-8"?>
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
							<ColorProcessing>{color_processing}</ColorProcessing>
							<Resolution>
								<Width>{args.resolution}</Width>
								<Height>{args.resolution}</Height>
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
</SOAP-ENV:Envelope>'''
        
        resp = send_request(dev, xml_create_job, "CreateScanJobResponseType", timeout=60000)
        xml_body = get_xml_body(resp).decode('utf-8', errors='replace')
        
        job_match = re.search(r'<JobId>(\d+)</JobId>', xml_body)
        if not job_match:
            print("Error: Failed to create scan job. Raw Response:")
            print(resp.decode('utf-8', errors='replace'))
            print("Decoded xml_body:")
            print(repr(xml_body))
            sys.exit(1)
        
        job_id = int(job_match.group(1))
        print(f"Scan Job created with ID: {job_id}")

        # Introduce a delay to let the scanner warm up and initialize the job
        print("Waiting 2.0s for scanner head to initialize job...")
        time.sleep(2.0)

        # 3. Retrieve Image
        print("Starting scan and retrieving image stream (this will take a moment)...")
        xml_retrieve = f'''<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope
	xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
	xmlns:SOAP-ENC="http://www.w3.org/2003/05/soap-encoding"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xmlns:xsd="http://www.w3.org/2001/XMLSchema"
	xmlns:wscn="http://tempuri.org/wscn.xsd">
	<SOAP-ENV:Body>
		<wscn:RetrieveImageRequest>
			<JobId>{job_id}</JobId>
			<JobToken></JobToken>
			<DocumentDescription></DocumentDescription>
		</wscn:RetrieveImageRequest>
	</SOAP-ENV:Body>
</SOAP-ENV:Envelope>'''
        
        # Write Retrieve Request
        payload = make_chunked_http_post(xml_retrieve)
        print(f"DEBUG RetrieveImage: Writing {len(payload)} bytes to EP_OUT...")
        bytes_written = dev.write(EP_OUT, payload, timeout=5000)
        print(f"DEBUG RetrieveImage: Successfully wrote {bytes_written} bytes.")
        
        # Read response using expected tag matching with a long timeout (60s) for scanning to finish
        image_data_raw = read_latest_response(dev, "RetrieveImageRequestResponse", timeout=60000, is_image=True)
        print(f"Transfer finished. Received {len(image_data_raw)} bytes.")

        # 4. Parse DIME response
        print("Parsing DIME image container...")
        records = parse_dime(image_data_raw)
        print(f"DEBUG: Parsed {len(records)} DIME records:")
        for idx, r in enumerate(records):
            print(f"  Record {idx}: ID={r['id']}, Type={r['type']}, Data size={len(r['data'])} bytes")
        
        # Reconstruct JPEG from DIME records (concatenating all chunks)
        jpeg_data = bytearray()
        found_image = False
        for r in records:
            if b"image" in r['type'] or b"jfif" in r['type'] or r['type'] == b'image/jfif':
                found_image = True
                jpeg_data.extend(r['data'])
            elif found_image and r['type'] == b'' and r['id'] == b'':
                # Subsequent chunk of the image
                jpeg_data.extend(r['data'])
            elif found_image:
                # Encountered another non-empty type record after the image started, stop concatenating
                break
                
        if not found_image:
            print("Error: Could not find image payload in the scan response.")
            print("\n--- Raw Un-chunked Response Body ---")
            print(get_xml_body(image_data_raw).decode('utf-8', errors='replace'))
            print("-------------------------------------")
            sys.exit(1)
            
        # Save image data
        with open(args.output, 'wb') as f:
            f.write(bytes(jpeg_data))
        print(f"Success! Scanned image saved to: {args.output} (total size: {len(jpeg_data)} bytes)")

    except Exception as e:
        print(f"Error during scan execution: {e}")
        sys.exit(1)
    finally:
        if job_id is not None:
            print("Cleaning up job on scanner...")
            xml_cancel = f'''<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope
	xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope"
	xmlns:SOAP-ENC="http://www.w3.org/2003/05/soap-encoding"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xmlns:xsd="http://www.w3.org/2001/XMLSchema"
	xmlns:wscn="http://tempuri.org/wscn.xsd">
	<SOAP-ENV:Body>
		<wscn:CancelJobRequest>
			<JobId>{job_id}</JobId>
			<JobToken></JobToken>
			<DocumentDescription></DocumentDescription>
		</wscn:CancelJobRequest>
	</SOAP-ENV:Body>
</SOAP-ENV:Envelope>'''
            try:
                send_request(dev, xml_cancel, "CancelRequestResponseType")
            except Exception as ce:
                print(f"Warning: Failed to cancel job {job_id}: {ce}")
        usb.util.release_interface(dev, INTERFACE_NUM)
        print("Released scanner interface.")

if __name__ == "__main__":
    main()
