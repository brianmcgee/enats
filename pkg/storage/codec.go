package storage

import (
	"bytes"
	"encoding/binary"
	"github.com/juju/errors"
)

func encodeNumber(number uint64) ([]byte, error) {
	numberBuf := new(bytes.Buffer)
	if err := binary.Write(numberBuf, binary.BigEndian, number); err != nil {
		return nil, errors.Annotate(err, "failed to encode number")
	}
	return numberBuf.Bytes(), nil
}

func mustEncodeNumber(number uint64) []byte {
	b, err := encodeNumber(number)
	if err != nil {
		panic(err)
	}
	return b
}

func decodeNumber(b []byte) (uint64, error) {
	buf := bytes.NewReader(b)
	var number uint64
	if err := binary.Read(buf, binary.BigEndian, &number); err != nil {
		return 0, errors.Annotate(err, "failed to decode number")
	}
	return number, nil
}

func mustDecodeNumber(b []byte) uint64 {
	n, err := decodeNumber(b)
	if err != nil {
		panic(err)
	}
	return n
}
