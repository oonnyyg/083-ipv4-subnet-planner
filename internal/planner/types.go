package planner

import (
	"fmt"
	"net"
)

// Task is the input JSON document.
type Task struct {
	Pools    []Pool    `json:"pools"`
	Reserved []Reserve `json:"reserved"`
	Requests []Request `json:"requests"`
}

type Pool struct {
	ID   string `json:"id"`
	CIDR string `json:"cidr"`
}

type Reserve struct {
	ID   string `json:"id"`
	Pool string `json:"pool"`
	CIDR string `json:"cidr"`
}

type Request struct {
	ID     string `json:"id"`
	Pool   string `json:"pool"`
	Prefix int    `json:"prefix"`
}

// Plan is the output JSON document.
type Plan struct {
	Allocations []Allocation `json:"allocations"`
	Pools       []PoolResult `json:"pools"`
	Stats       Stats        `json:"stats"`
}

type Allocation struct {
	Request string `json:"request"`
	Pool    string `json:"pool"`
	CIDR    string `json:"cidr"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type PoolResult struct {
	ID        string  `json:"id"`
	Remaining []Block `json:"remaining"`
}

type Block struct {
	CIDR  string `json:"cidr"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type Stats struct {
	Total     uint64 `json:"total"`
	Reserved  uint64 `json:"reserved"`
	Allocated uint64 `json:"allocated"`
	Remaining uint64 `json:"remaining"`
}

// ObjectError identifies the offending input object.
type ObjectError struct {
	Kind string // "pool", "reserved", "request"
	ID   string
	Msg  string
}

func (e *ObjectError) Error() string {
	return fmt.Sprintf("%s %q: %s", e.Kind, e.ID, e.Msg)
}

func objErr(kind, id, format string, args ...any) *ObjectError {
	return &ObjectError{Kind: kind, ID: id, Msg: fmt.Sprintf(format, args...)}
}

// parseCIDR parses an IPv4 CIDR, rejecting non-network addresses.
func parseCIDR(kind, id, s string) (uint32, int, error) {
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return 0, 0, objErr(kind, id, "invalid CIDR %q", s)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, 0, objErr(kind, id, "not an IPv4 CIDR %q", s)
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return 0, 0, objErr(kind, id, "not an IPv4 CIDR %q", s)
	}
	addr := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
	network := addr & mask(ones)
	if network != addr {
		return 0, 0, objErr(kind, id, "CIDR %q has non-zero host bits; use the network address", s)
	}
	return addr, ones, nil
}

func mask(ones int) uint32 {
	if ones == 0 {
		return 0
	}
	return ^uint32(0) << (32 - ones)
}

func sizeOf(ones int) uint64 {
	return uint64(1) << (32 - ones)
}

func ipString(v uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", v>>24, v>>16&0xff, v>>8&0xff, v&0xff)
}
