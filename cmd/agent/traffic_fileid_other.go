//go:build !linux

package main

import "os"

// trafficFileID is a file's identity. Outside Linux (development builds) the
// identity is not known; a rotation then reads as a truncation.
type trafficFileID struct {
	Dev uint64 `json:"dev"`
	Ino uint64 `json:"ino"`
}

func statTrafficFile(path string) (trafficFileID, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return trafficFileID{}, 0, err
	}
	return trafficFileID{}, info.Size(), nil
}
