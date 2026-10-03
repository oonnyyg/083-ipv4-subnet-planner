package planner

import (
	"fmt"
	"sort"
)

// Task is the JSON input: address pools, reserved blocks and requests.
type Task struct {
	Pools        []PoolInput        `json:"pools"`
	Reservations []ReservationInput `json:"reservations"`
	Requests     []RequestInput     `json:"requests"`
}

type PoolInput struct {
	ID   string `json:"id"`
	CIDR string `json:"cidr"`
}

type ReservationInput struct {
	ID     string `json:"id"`
	PoolID string `json:"pool"`
	CIDR   string `json:"cidr"`
}

type RequestInput struct {
	ID     string `json:"id"`
	PoolID string `json:"pool"`
	Prefix int    `json:"prefix"`
}

// Plan is the successful JSON output.
type Plan struct {
	Allocations []AllocationOutput `json:"allocations"`
	Pools       []PoolOutput       `json:"pools"`
	Statistics  Statistics         `json:"statistics"`
}

type AllocationOutput struct {
	RequestID string `json:"request"`
	PoolID    string `json:"pool"`
	CIDR      string `json:"cidr"`
	Start     string `json:"start"`
	End       string `json:"end"`
}

type PoolOutput struct {
	PoolID    string   `json:"pool"`
	CIDR      string   `json:"cidr"`
	Remaining []string `json:"remaining"`
}

type Statistics struct {
	Total     uint64 `json:"total"`
	Reserved  uint64 `json:"reserved"`
	Allocated uint64 `json:"allocated"`
	Remaining uint64 `json:"remaining"`
}

// Failure identifies the object that made planning impossible.
type Failure struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type interval struct {
	start uint64
	end   uint64
}

type poolState struct {
	id       string
	block    block
	reserved []block
	busy     []interval // reserved + already allocated, kept sorted/merged
	requests []RequestInput
}

// Run validates the task and produces a deterministic allocation plan.
func Run(t Task) (*Plan, *Failure) {
	if f := validate(t); f != nil {
		return nil, f
	}

	pools := make(map[string]*poolState)
	var order []string
	for _, p := range t.Pools {
		b, _ := parseCIDR(p.CIDR)
		ps := &poolState{id: p.ID, block: b}
		pools[p.ID] = ps
		order = append(order, p.ID)
	}
	for _, r := range t.Reservations {
		b, _ := parseCIDR(r.CIDR)
		ps := pools[r.PoolID]
		ps.reserved = append(ps.reserved, b)
		ps.busy = append(ps.busy, interval{b.base, b.end()})
	}
	for _, r := range t.Requests {
		pools[r.PoolID].requests = append(pools[r.PoolID].requests, r)
	}

	plan := &Plan{Allocations: []AllocationOutput{}, Pools: []PoolOutput{}}

	for _, id := range order {
		ps := pools[id]
		reqs := append([]RequestInput(nil), ps.requests...)
		sort.Slice(reqs, func(i, j int) bool {
			if reqs[i].Prefix != reqs[j].Prefix {
				return reqs[i].Prefix < reqs[j].Prefix
			}
			return reqs[i].ID < reqs[j].ID
		})

		for _, req := range reqs {
			size := uint64(1) << (32 - req.Prefix)
			b, ok := allocate(ps.block, ps.busy, size)
			if !ok {
				return nil, &Failure{
					Object: "request",
					ID:     req.ID,
					Reason: fmt.Sprintf("no free aligned /%d block available in pool %q", req.Prefix, ps.id),
				}
			}
			ps.busy = append(ps.busy, interval{b.base, b.end()})
			plan.Allocations = append(plan.Allocations, AllocationOutput{
				RequestID: req.ID,
				PoolID:    ps.id,
				CIDR:      b.cidr(),
				Start:     formatIPv4(b.base),
				End:       formatIPv4(b.end() - 1),
			})
		}

		remaining := freeCIDRs(ps.block, ps.busy)
		out := PoolOutput{PoolID: ps.id, CIDR: ps.block.cidr(), Remaining: []string{}}
		for _, b := range remaining {
			out.Remaining = append(out.Remaining, b.cidr())
		}
		plan.Pools = append(plan.Pools, out)
	}

	sort.Slice(plan.Allocations, func(i, j int) bool {
		return plan.Allocations[i].RequestID < plan.Allocations[j].RequestID
	})
	sort.Slice(plan.Pools, func(i, j int) bool {
		bi, _ := parseCIDR(plan.Pools[i].CIDR)
		bj, _ := parseCIDR(plan.Pools[j].CIDR)
		return bi.base < bj.base
	})

	for _, p := range t.Pools {
		b, _ := parseCIDR(p.CIDR)
		plan.Statistics.Total += b.size
	}
	for _, r := range t.Reservations {
		b, _ := parseCIDR(r.CIDR)
		plan.Statistics.Reserved += b.size
	}
	for _, a := range plan.Allocations {
		b, _ := parseCIDR(a.CIDR)
		plan.Statistics.Allocated += b.size
	}
	plan.Statistics.Remaining = plan.Statistics.Total - plan.Statistics.Reserved - plan.Statistics.Allocated

	return plan, nil
}

// allocate finds the lowest aligned free block of the given size that does
// not cross any busy interval. It performs a single ordered scan.
func allocate(pool block, busy []interval, size uint64) (block, bool) {
	sorted := mergedIntervals(busy)
	cursor := pool.base
	poolEnd := pool.end()
	for _, iv := range sorted {
		if b, ok := tryFit(cursor, iv.start, size, poolEnd); ok {
			return b, true
		}
		if iv.end > cursor {
			cursor = iv.end
		}
	}
	b, ok := tryFit(cursor, poolEnd, size, poolEnd)
	return b, ok
}

// tryFit searches the free gap [lo, hi) for the lowest aligned block.
func tryFit(lo, hi, size, poolEnd uint64) (block, bool) {
	if hi > poolEnd {
		hi = poolEnd
	}
	if lo >= hi || hi-lo < size {
		return block{}, false
	}
	cursor := lo
	for cursor+size <= hi {
		if cursor&(size-1) == 0 {
			return block{base: cursor, size: size}, true
		}
		cursor = (cursor | (size - 1)) + 1
	}
	return block{}, false
}

// freeCIDRs decomposes the free space of a pool into the minimum number of
// non-overlapping, sorted CIDR blocks.
func freeCIDRs(pool block, busy []interval) []block {
	sorted := mergedIntervals(busy)
	var out []block
	cursor := pool.base
	end := pool.end()
	for _, iv := range sorted {
		out = append(out, fillGap(cursor, min64(iv.start, end))...)
		if iv.end > cursor {
			cursor = iv.end
		}
	}
	out = append(out, fillGap(cursor, end)...)
	return out
}

// fillGap greedily emits the largest possible aligned CIDR blocks covering
// [lo, hi); it runs in time proportional to the number of emitted blocks.
func fillGap(lo, hi uint64) []block {
	var out []block
	for lo < hi {
		size := highestPowerOfTwo(hi - lo)
		for lo&(size-1) != 0 {
			size >>= 1
		}
		out = append(out, block{base: lo, size: size})
		lo += size
	}
	return out
}

func highestPowerOfTwo(n uint64) uint64 {
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n - (n >> 1)
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func mergedIntervals(ivs []interval) []interval {
	if len(ivs) == 0 {
		return nil
	}
	sorted := append([]interval(nil), ivs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })
	merged := []interval{sorted[0]}
	for _, iv := range sorted[1:] {
		last := &merged[len(merged)-1]
		if iv.start <= last.end {
			if iv.end > last.end {
				last.end = iv.end
			}
		} else {
			merged = append(merged, iv)
		}
	}
	return merged
}
