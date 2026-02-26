//go:build !riscv64

package fastdns

import (
	"errors"
)

var (
	ErrTruncated = errors.New("dns packet truncated")
	ErrInvalid   = errors.New("invalid dns label")
)

// ParseDNSPacket extracts domain names from DNS queries using pure Go.
// This is the fallback implementation for non-RISC-V architectures.
//
// On RISC-V platforms with vector extensions, see parse_riscv64.go for
// the optimized assembly implementation.
func ParseDNSPacket(packet []byte) (domain string, err error) {
	// DNS question starts at offset 12 (header is fixed 12 bytes)
	const headerSize = 12

	if len(packet) < headerSize {
		return "", ErrTruncated
	}

	// Pure Go implementation (not optimized)
	// This is just a placeholder - the real parsing is in the main DNS handler
	return "", errors.New("fastdns: pure go implementation not yet implemented")
}

// IsVectorOptimized returns false on non-RISC-V platforms.
func IsVectorOptimized() bool {
	return false
}

// VectorLength returns 0 on platforms without vector support.
func VectorLength() int {
	return 0
}
