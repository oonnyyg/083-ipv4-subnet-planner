package planner

import "sort"

func fail(object, id, reason string) *Failure {
	return &Failure{Object: object, ID: id, Reason: reason}
}

func validate(t Task) *Failure {
	if len(t.Pools) == 0 {
		return fail("task", "", "at least one pool is required")
	}

	poolByID := make(map[string]block, len(t.Pools))
	var poolBlocks []block
	seenPool := make(map[string]bool)
	for _, p := range t.Pools {
		if p.ID == "" {
			return fail("pool", "", "pool id must not be empty")
		}
		if seenPool[p.ID] {
			return fail("pool", p.ID, "duplicate pool id")
		}
		seenPool[p.ID] = true
		b, err := parseCIDR(p.CIDR)
		if err != nil {
			return fail("pool", p.ID, "invalid cidr: "+err.Error())
		}
		poolByID[p.ID] = b
		poolBlocks = append(poolBlocks, b)
	}
	sort.Slice(poolBlocks, func(i, j int) bool { return poolBlocks[i].base < poolBlocks[j].base })
	for i := 1; i < len(poolBlocks); i++ {
		if poolBlocks[i-1].overlaps(poolBlocks[i]) {
			return fail("pool", "", "pools must not overlap: "+
				poolBlocks[i-1].cidr()+" and "+poolBlocks[i].cidr())
		}
	}

	seenRes := make(map[string]bool)
	resByPool := make(map[string][]block)
	for _, r := range t.Reservations {
		if r.ID == "" {
			return fail("reservation", "", "reservation id must not be empty")
		}
		if seenRes[r.ID] {
			return fail("reservation", r.ID, "duplicate reservation id")
		}
		seenRes[r.ID] = true
		pb, ok := poolByID[r.PoolID]
		if !ok {
			return fail("reservation", r.ID, "unknown pool: "+r.PoolID)
		}
		b, err := parseCIDR(r.CIDR)
		if err != nil {
			return fail("reservation", r.ID, "invalid cidr: "+err.Error())
		}
		if !pb.contains(b) {
			return fail("reservation", r.ID, "reservation "+b.cidr()+
				" is not fully inside pool "+r.PoolID+" ("+pb.cidr()+")")
		}
		for _, other := range resByPool[r.PoolID] {
			if other.overlaps(b) {
				return fail("reservation", r.ID, "overlaps reservation "+
					other.cidr()+" in pool "+r.PoolID)
			}
		}
		resByPool[r.PoolID] = append(resByPool[r.PoolID], b)
	}

	seenReq := make(map[string]bool)
	for _, r := range t.Requests {
		if r.ID == "" {
			return fail("request", "", "request id must not be empty")
		}
		if seenReq[r.ID] {
			return fail("request", r.ID, "duplicate request id")
		}
		seenReq[r.ID] = true
		if _, ok := poolByID[r.PoolID]; !ok {
			return fail("request", r.ID, "unknown pool: "+r.PoolID)
		}
		if r.Prefix < 0 || r.Prefix > 32 {
			return fail("request", r.ID, "prefix length must be in [0,32]")
		}
	}
	return nil
}
