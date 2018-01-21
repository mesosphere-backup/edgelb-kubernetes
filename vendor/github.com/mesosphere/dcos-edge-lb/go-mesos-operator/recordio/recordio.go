package recordio

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
)

const recordDelimiter byte = '\n'
const recordDelimiterStr string = string(recordDelimiter)

// Parser for recordio
type Parser interface {
	io.ReadCloser

	Record() ([]byte, error)
}

type stream struct {
	rd *bufio.Reader // The input stream
	cs io.Closer     // Used to close the input stream
}

// NewParser returns a new recordio Parser
func NewParser(r io.ReadCloser) Parser {
	return &stream{
		rd: bufio.NewReader(r),
		cs: r,
	}
}

// Record returns a record.
//
// Record() and Read() cannot be used in tandem.
func (s *stream) Record() (p []byte, err error) {
	header, err := s.rd.ReadBytes(recordDelimiter)
	if err != nil {
		return nil, fmt.Errorf("error reading header: %s, got \"%s\"", err, string(header))
	}
	recordSizeStr := string(bytes.TrimRight(header, recordDelimiterStr))
	recordSize, err := strconv.ParseUint(recordSizeStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("error parsing header: %s", err)
	}

	var rec []byte
	var bytesRead uint64
	for bytesRead != recordSize {
		tmprec := make([]byte, recordSize-bytesRead)
		readSizeInt, err := s.rd.Read(tmprec)
		if err != nil {
			return nil, fmt.Errorf("error reading record: %s", err)
		}
		bytesRead += uint64(readSizeInt)
		rec = append(rec, tmprec[:readSizeInt]...)
	}
	return rec, nil
}

// Read is identical to io.Parser.Read()
//
// Record() and Read() cannot be used in tandem.
func (s *stream) Read(p []byte) (n int, err error) {
	return s.rd.Read(p)
}

// Close will close the underlying reader.
//
// It needs to be closed even when an error occurs while reading.
func (s *stream) Close() error {
	return s.cs.Close()
}
