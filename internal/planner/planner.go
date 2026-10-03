package planner

import (
	"fmt"
	"sort"
)

type interval struct {
	lo, hi uint32 // inclusive
}

type poolState struct {
	id       string
	lo, hi   uint32
	reserved uint64
	free     []interval // sorted, non-overlapping
}

// Plan validates the task and produces a full allocation plan.
func PlanTask(t *Task) (*Plan, error) {
	pools, err := buildPools(t)
	if err != nil {
		return nil, err
	}
	reqs, err := validateRequests(t, pools)
	if err != nil {
		return nil, err
	}

	plan := &Plan{Stats: Stats{}}
	poolIDs := make([]string, 0, len(pools))
	for id := range pools {
		poolIDs = append(poolIDs, id)
	}
	sort.Strings(poolIDs)

	for _, id := range poolIDs {
		p := pools[id]
		plan.Stats.Total += uint64(p.hi) - uint64(p.lo) + 1
		plan.Stats.Reserved += p.reserved
	}

	for _, r := range reqs {
		p := pools[r.Pool]
		size := sizeOf(r.Prefix)
		iv, start, ok := findFit(p.free, size)
		if !ok {
			return nil, objErr("request", r.ID,
				"cannot satisfy prefix /%d in pool %q: no aligned free block of %d addresses",
				r.Prefix, r.Pool, size)
		}
		end := start + uint32(size) - 1
		p.free = carve(p.free, iv, start, end)
		plan.Allocations = append(plan.Allocations, Allocation{
			Request: r.ID,
			Pool:    r.Pool,
			CIDR:    fmt.Sprintf("%s/%d", ipString(start), r.Prefix),
			Start:   ipString(start),
			End:     ipString(end),
		})
		plan.Stats.Allocated += size
	}

	for _, id := range poolIDs {
		p := pools[id]
		pr := PoolResult{ID: p.id, Remaining: []Block{}}
		for _, iv := range p.free {
			plan.Stats.Remaining += uint64(iv.hi) - uint64(iv.lo) + 1
			for _, b := range toCIDRs(iv.lo, iv.hi) {
				pr.Remaining = append(pr.Remaining, b)
			}
		}
		plan.Pools = append(plan.Pools, pr)
	}
	return plan, nil
}

// findFit returns the index of the free interval and the smallest aligned
// start address that fits the requested size.
func findFit(free []interval, size uint64) (int, uint32, bool) {
	for i, iv := range free {
		start := alignUp(iv.lo, size)
		if uint64(start)+size-1 <= uint64(iv.hi) {
			return i, start, true
		}
	}
	return 0, 0, false
}

func alignUp(addr uint32, size uint64) uint32 {
	m := size - 1
	return uint32((uint64(addr) + m) &^ m)
}

// carve removes [start,end] from free[iv], splitting the interval.
func carve(free []interval, iv int, start, end uint32) []interval {
	cur := free[iv]
	var out []interval
	if cur.lo < start {
		out = append(out, interval{cur.lo, start - 1})
	}
	if end < cur.hi {
		out = append(out, interval{end + 1, cur.hi})
	}
	return append(free[:iv], append(out, free[iv+1:]...)...)
}

// toCIDRs decomposes [lo,hi] into the minimal set of CIDR blocks,
// sorted by network address.
func toCIDRs(lo, hi uint32) []Block {
	var blocks []Block
	for lo <= hi {
		maxSize := uint64(1) << trailingZeros(lo)
		remain := uint64(hi) - uint64(lo) + 1
		for maxSize > remain {
			maxSize >>= 1
		}
		prefix := 32 - log2u64(maxSize)
		end := lo + uint32(maxSize) - 1
		blocks = append(blocks, Block{
			CIDR:  fmt.Sprintf("%s/%d", ipString(lo), prefix),
			Start: ipString(lo),
			End:   ipString(end),
		})
		if end == ^uint32(0) {
			break
		}
		lo = end + 1
	}
	return blocks
}

func trailingZeros(v uint32) uint {
	if v == 0 {
		return 32
	}
	n := uint(0)
	for v&1 == 0 {
		v >>= 1
		n++
	}
	return n
}

func log2u64(v uint64) int {
	n := 0
	for v > 1 {
		v >>= 1
		n++
	}
	return n
}
