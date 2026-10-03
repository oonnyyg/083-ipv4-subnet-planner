package planner

import "sort"

func buildPools(t *Task) (map[string]*poolState, error) {
	pools := map[string]*poolState{}
	order := []string{}
	for _, p := range t.Pools {
		if _, dup := pools[p.ID]; dup {
			return nil, objErr("pool", p.ID, "duplicate pool id")
		}
		lo, ones, err := parseCIDR("pool", p.ID, p.CIDR)
		if err != nil {
			return nil, err
		}
		hi := lo + uint32(sizeOf(ones)) - 1
		ps := &poolState{id: p.ID, lo: lo, hi: hi, free: []interval{{lo, hi}}}
		pools[p.ID] = ps
		order = append(order, p.ID)
	}
	// Pools must not overlap.
	sorted := make([]*poolState, 0, len(pools))
	for _, id := range order {
		sorted = append(sorted, pools[id])
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].lo < sorted[j].lo })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].lo <= sorted[i-1].hi {
			return nil, objErr("pool", sorted[i].id,
				"pool overlaps with pool %q", sorted[i-1].id)
		}
	}
	// Reserved ranges.
	seen := map[string]bool{}
	for _, r := range t.Reserved {
		if seen[r.ID] {
			return nil, objErr("reserved", r.ID, "duplicate reserved id")
		}
		seen[r.ID] = true
		p, ok := pools[r.Pool]
		if !ok {
			return nil, objErr("reserved", r.ID, "unknown pool %q", r.Pool)
		}
		lo, ones, err := parseCIDR("reserved", r.ID, r.CIDR)
		if err != nil {
			return nil, err
		}
		hi := lo + uint32(sizeOf(ones)) - 1
		if lo < p.lo || hi > p.hi {
			return nil, objErr("reserved", r.ID,
				"reserved range %s is not fully inside pool %q", r.CIDR, r.Pool)
		}
		// Remove from free list; overlap with existing reserved shows up here.
		idx := -1
		for i, iv := range p.free {
			if iv.lo <= lo && hi <= iv.hi {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, objErr("reserved", r.ID,
				"reserved range %s duplicates or overlaps another reserved range in pool %q", r.CIDR, r.Pool)
		}
		p.free = carve(p.free, idx, lo, hi)
		p.reserved += sizeOf(ones)
	}
	return pools, nil
}

func validateRequests(t *Task, pools map[string]*poolState) ([]Request, error) {
	seen := map[string]bool{}
	for _, r := range t.Requests {
		if seen[r.ID] {
			return nil, objErr("request", r.ID, "duplicate request id")
		}
		seen[r.ID] = true
		if _, ok := pools[r.Pool]; !ok {
			return nil, objErr("request", r.ID, "unknown pool %q", r.Pool)
		}
		if r.Prefix < 0 || r.Prefix > 32 {
			return nil, objErr("request", r.ID, "prefix %d out of range 0..32", r.Prefix)
		}
	}
	reqs := make([]Request, len(t.Requests))
	copy(reqs, t.Requests)
	// Deterministic order: pool id, then prefix ascending, then request id.
	sort.SliceStable(reqs, func(i, j int) bool {
		if reqs[i].Pool != reqs[j].Pool {
			return reqs[i].Pool < reqs[j].Pool
		}
		if reqs[i].Prefix != reqs[j].Prefix {
			return reqs[i].Prefix < reqs[j].Prefix
		}
		return reqs[i].ID < reqs[j].ID
	})
	return reqs, nil
}
