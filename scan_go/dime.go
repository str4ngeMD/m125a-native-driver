package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
)

// DimeRecord represents a single un-chunked DIME record.
type DimeRecord struct {
	Flags byte
	ID    string
	Type  string
	Data  []byte
}

// DecodeHTTPChunked removes the HTTP chunked transfer encoding wrapping.
func DecodeHTTPChunked(data []byte) ([]byte, error) {
	headerSeparator := []byte("\r\n\r\n")
	idx := bytes.Index(data, headerSeparator)
	var body []byte
	if idx != -1 {
		body = data[idx+4:]
	} else {
		body = data
	}

	var decoded []byte
	offset := 0
	for offset < len(body) {
		lineEnd := bytes.Index(body[offset:], []byte("\r\n"))
		if lineEnd == -1 {
			break
		}
		chunkSizeStr := string(bytes.TrimSpace(body[offset : offset+lineEnd]))
		if chunkSizeStr == "" {
			offset += lineEnd + 2
			continue
		}

		chunkSize, err := strconv.ParseInt(chunkSizeStr, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid chunk size hex '%s': %v", chunkSizeStr, err)
		}
		if chunkSize == 0 {
			break
		}

		offset += lineEnd + 2
		if offset+int(chunkSize) > len(body) {
			// Incomplete chunk, read what's available
			decoded = append(decoded, body[offset:]...)
			break
		}
		decoded = append(decoded, body[offset:offset+int(chunkSize)]...)
		offset += int(chunkSize) + 2
	}
	return decoded, nil
}

// ParseDime parses the un-chunked DIME body and returns a list of DIME records.
// It handles DIME record chunking (CF flag) automatically by concatenating chunk data.
func ParseDime(data []byte) ([]DimeRecord, error) {
	var records []DimeRecord
	var activeChunk *DimeRecord
	idx := 0

	for idx < len(data) {
		if idx+12 > len(data) {
			break
		}
		header := data[idx : idx+12]
		flags := header[0]
		cf := (flags & 0x20) != 0

		optLen := binary.BigEndian.Uint16(header[2:4])
		idLen := binary.BigEndian.Uint16(header[4:6])
		typeLen := binary.BigEndian.Uint16(header[6:8])
		dataLen := binary.BigEndian.Uint32(header[8:12])

		optPadded := int((optLen + 3) &^ 3)
		idPadded := int((idLen + 3) &^ 3)
		typePadded := int((typeLen + 3) &^ 3)
		dataPadded := int((dataLen + 3) &^ 3)

		idx += 12

		if idx+optPadded+idPadded+typePadded+dataPadded > len(data) {
			return nil, fmt.Errorf("unexpected EOF while parsing DIME record contents")
		}

		// Options data (ignored)
		idx += optPadded

		idData := string(data[idx : idx+int(idLen)])
		idx += idPadded

		typeData := string(data[idx : idx+int(typeLen)])
		idx += typePadded

		dataData := data[idx : idx+int(dataLen)]
		idx += dataPadded

		if activeChunk != nil {
			activeChunk.Data = append(activeChunk.Data, dataData...)
			if !cf {
				// Final chunk in the sequence
				records = append(records, *activeChunk)
				activeChunk = nil
			}
		} else {
			if cf {
				activeChunk = &DimeRecord{
					Flags: flags,
					ID:    idData,
					Type:  typeData,
					Data:  append([]byte(nil), dataData...),
				}
			} else {
				records = append(records, DimeRecord{
					Flags: flags,
					ID:    idData,
					Type:  typeData,
					Data:  append([]byte(nil), dataData...),
				})
			}
		}
	}

	if activeChunk != nil {
		return nil, fmt.Errorf("unterminated DIME chunk sequence")
	}

	return records, nil
}
