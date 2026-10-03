package planner

import (
	"errors"
	"strconv"
	"strings"
)

// block represents an IPv4 CIDR block using uint64 arithmetic so that
// the full 0.0.0.0/0 range (2^32 addresses) does not overflow.
type block struct {
	base uint64
	size uint64 // always a power of two, 1 .. 2^32
}

func (b block) end() uint64 { return b.base + b.size }

func (b block) contains(o block) bool {
	return b.base <= o.base && o.end() <= b.end()
}

func (b block) overlaps(o block) bool {
	return b.base < o.end() && o.base < b.end()
}

func (b block) prefixLen() int {
	n := b.size
	p := 0
	for n > 1 {
		n >>= 1
		p++
	}
	return 32 - p
}

// parseCIDR parses an IPv4 CIDR. Host bits must be zero; otherwise the
// input is rejected instead of being silently truncated.
func parseCIDR(s string) (block, error) {
	idx := strings.IndexByte(s, '/')
	if idx < 0 {
		return block{}, errors.New("missing '/' prefix separator")
	}
	addrStr, prefixStr := s[:idx], s[idx+1:]
	ip, err := parseIPv4(addrStr)
	if err != nil {
		return block{}, err
	}
	prefix, err := strconv.Atoi(prefixStr)
	if err != nil || prefix < 0 || prefix > 32 {
		return block{}, errors.New("prefix length must be an integer in [0,32]")
	}
	size := uint64(1) << (32 - prefix)
	mask := size - 1
	if uint64(ip)&mask != 0 {
		return block{}, errors.New("host bits are non-zero; network address required")
	}
	return block{base: uint64(ip), size: size}, nil
}

func parseIPv4(s string) (uint32, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return 0, errors.New("IPv4 address must have 4 dotted parts")
	}
	var ip uint32
	for _, p := range parts {
		if p == "" || len(p) > 3 || (len(p) > 1 && p[0] == '0') {
			return 0, errors.New("invalid IPv4 octet: " + p)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return 0, errors.New("invalid IPv4 octet: " + p)
		}
		ip = ip<<8 | uint32(n)
	}
	return ip, nil
}

func formatIPv4(addr uint64) string {
	return strconv.FormatUint(addr>>24, 10) + "." +
		strconv.FormatUint((addr>>16)&0xff, 10) + "." +
		strconv.FormatUint((addr>>8)&0xff, 10) + "." +
		strconv.FormatUint(addr&0xff, 10)
}

func (b block) cidr() string {
	return formatIPv4(b.base) + "/" + strconv.Itoa(b.prefixLen())
}
