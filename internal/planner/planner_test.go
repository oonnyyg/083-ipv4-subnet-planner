package planner

import "testing"

func TestAllocationOrderAndAlignment(t *testing.T) {
	task := Task{
		Pools: []PoolInput{{ID: "p1", CIDR: "10.0.0.0/24"}},
		Requests: []RequestInput{
			{ID: "small", PoolID: "p1", Prefix: 32},
			{ID: "big", PoolID: "p1", Prefix: 26},
		},
	}
	plan, f := Run(task)
	if f != nil {
		t.Fatalf("unexpected failure: %v", f)
	}
	got := map[string]string{}
	for _, a := range plan.Allocations {
		got[a.RequestID] = a.CIDR
	}
	// /26 is processed before /32 because prefix length ascending.
	if got["big"] != "10.0.0.0/26" {
		t.Fatalf("big = %s, want 10.0.0.0/26", got["big"])
	}
	if got["small"] != "10.0.0.64/32" {
		t.Fatalf("small = %s, want 10.0.0.64/32", got["small"])
	}
	if plan.Statistics.Total != 256 || plan.Statistics.Allocated != 65 ||
		plan.Statistics.Remaining != 191 || plan.Statistics.Reserved != 0 {
		t.Fatalf("bad stats: %+v", plan.Statistics)
	}
}

func TestTieBreakByRequestID(t *testing.T) {
	task := Task{
		Pools: []PoolInput{{ID: "p1", CIDR: "10.0.0.0/29"}},
		Requests: []RequestInput{
			{ID: "zeta", PoolID: "p1", Prefix: 30},
			{ID: "alpha", PoolID: "p1", Prefix: 30},
		},
	}
	plan, f := Run(task)
	if f != nil {
		t.Fatal(f)
	}
	if plan.Allocations[0].RequestID != "alpha" || plan.Allocations[0].CIDR != "10.0.0.0/30" {
		t.Fatalf("alpha allocation wrong: %+v", plan.Allocations[0])
	}
	if plan.Allocations[1].RequestID != "zeta" || plan.Allocations[1].CIDR != "10.0.0.4/30" {
		t.Fatalf("zeta allocation wrong: %+v", plan.Allocations[1])
	}
}

func TestReservedBlocksFragmentSpace(t *testing.T) {
	// /28 pool; reserve 10.0.0.4/30 and 10.0.0.10/31 leaving fragments.
	task := Task{
		Pools:        []PoolInput{{ID: "p1", CIDR: "10.0.0.0/28"}},
		Reservations: []ReservationInput{{ID: "r1", PoolID: "p1", CIDR: "10.0.0.4/30"}},
		Requests: []RequestInput{
			{ID: "h1", PoolID: "p1", Prefix: 32},
			{ID: "h2", PoolID: "p1", Prefix: 32},
			{ID: "h3", PoolID: "p1", Prefix: 32},
			{ID: "h4", PoolID: "p1", Prefix: 32},
		},
	}
	plan, f := Run(task)
	if f != nil {
		t.Fatal(f)
	}
	want := []string{"10.0.0.0/32", "10.0.0.1/32", "10.0.0.2/32", "10.0.0.3/32"}
	for i, a := range plan.Allocations {
		if a.CIDR != want[i] {
			t.Fatalf("alloc %d = %s, want %s", i, a.CIDR, want[i])
		}
	}
	// Remaining [8,16) after reservation [4,8) and allocations [0,4):
	// one aligned /29 is exactly the minimal representation.
	wantRem := []string{"10.0.0.8/29"}
	if len(plan.Pools[0].Remaining) != len(wantRem) {
		t.Fatalf("remaining = %v, want %v", plan.Pools[0].Remaining, wantRem)
	}
	for i, r := range plan.Pools[0].Remaining {
		if r != wantRem[i] {
			t.Fatalf("remaining[%d] = %s, want %s", i, r, wantRem[i])
		}
	}
	if plan.Statistics.Total != 16 || plan.Statistics.Reserved != 4 ||
		plan.Statistics.Allocated != 4 || plan.Statistics.Remaining != 8 {
		t.Fatalf("bad stats: %+v", plan.Statistics)
	}
}

func TestNoMovesAroundReservation(t *testing.T) {
	// /29 pool with the middle /30 reserved: free gaps are [0,4) and [8,8)? => [8,8) wait:
	// reserve 10.0.0.4/30 -> free [0,4) and [8,8)?? /29 has 8 addrs [0,8): free [8,8) empty.
	task := Task{
		Pools:        []PoolInput{{ID: "p1", CIDR: "10.0.0.0/29"}},
		Reservations: []ReservationInput{{ID: "r1", PoolID: "p1", CIDR: "10.0.0.4/30"}},
		Requests: []RequestInput{
			{ID: "a", PoolID: "p1", Prefix: 32},
			{ID: "b", PoolID: "p1", Prefix: 32},
			{ID: "c", PoolID: "p1", Prefix: 32},
			{ID: "d", PoolID: "p1", Prefix: 32},
			{ID: "e", PoolID: "p1", Prefix: 32},
		},
	}
	_, f := Run(task)
	if f == nil || f.ID != "e" {
		t.Fatalf("expected failure on request e, got %v", f)
	}
}

func TestLargePoolConservationAndNoExpansion(t *testing.T) {
	// A /0 pool is the entire 2^32 space; allocations must stay cheap and
	// statistics must conserve without per-address iteration.
	task := Task{
		Pools: []PoolInput{
			{ID: "big", CIDR: "0.0.0.0/0"},
			{ID: "small", CIDR: "200.0.0.0/24"},
		},
		Reservations: []ReservationInput{{ID: "r1", PoolID: "big", CIDR: "10.0.0.0/8"}},
		Requests: []RequestInput{
			{ID: "x", PoolID: "big", Prefix: 0},
			{ID: "y", PoolID: "small", Prefix: 24},
		},
	}
	_, f := Run(task)
	if f == nil {
		t.Fatal("expected failure: /0 request cannot fit with any reservation")
	}

	task = Task{
		Pools:        []PoolInput{{ID: "big", CIDR: "0.0.0.0/0"}},
		Reservations: []ReservationInput{{ID: "r1", PoolID: "big", CIDR: "10.0.0.0/8"}},
		Requests: []RequestInput{
			{ID: "x", PoolID: "big", Prefix: 8},
			{ID: "y", PoolID: "big", Prefix: 8},
		},
	}
	plan, f := Run(task)
	if f != nil {
		t.Fatal(f)
	}
	alloc := map[string]string{}
	for _, a := range plan.Allocations {
		alloc[a.RequestID] = a.CIDR
	}
	if alloc["x"] != "0.0.0.0/8" || alloc["y"] != "1.0.0.0/8" {
		t.Fatalf("unexpected allocations: %v", alloc)
	}
	if plan.Statistics.Total != 1<<32 ||
		plan.Statistics.Reserved != 1<<24 ||
		plan.Statistics.Allocated != 2<<24 ||
		plan.Statistics.Remaining != (1<<32)-(3<<24) {
		t.Fatalf("bad stats: %+v", plan.Statistics)
	}
	// Pools must never be merged across boundaries; remaining count is small
	// (3 /8-ish splits plus the big tail /1 .. etc), just verify sorted order.
	rem := plan.Pools[0].Remaining
	for i := 1; i < len(rem); i++ {
		b0, _ := parseCIDR(rem[i-1])
		b1, _ := parseCIDR(rem[i])
		if b0.base >= b1.base {
			t.Fatalf("remaining not sorted: %s then %s", rem[i-1], rem[i])
		}
		if b0.overlaps(b1) {
			t.Fatalf("remaining overlap: %s %s", rem[i-1], rem[i])
		}
	}
}

func TestRejectHostBits(t *testing.T) {
	cases := []Task{
		{Pools: []PoolInput{{ID: "p", CIDR: "10.0.0.1/24"}}},
		{
			Pools:        []PoolInput{{ID: "p", CIDR: "10.0.0.0/24"}},
			Reservations: []ReservationInput{{ID: "r", PoolID: "p", CIDR: "10.0.0.5/30"}},
		},
	}
	for i, task := range cases {
		if _, f := Run(task); f == nil {
			t.Fatalf("case %d: expected host-bit failure", i)
		}
	}
}

func TestRejectOverlapsAndBadReferences(t *testing.T) {
	cases := []Task{
		{
			Pools: []PoolInput{
				{ID: "a", CIDR: "10.0.0.0/23"},
				{ID: "b", CIDR: "10.0.1.0/24"},
			},
		},
		{
			Pools:        []PoolInput{{ID: "p", CIDR: "10.0.0.0/24"}},
			Reservations: []ReservationInput{{ID: "r", PoolID: "p", CIDR: "10.0.1.0/24"}},
		},
		{
			Pools: []PoolInput{{ID: "p", CIDR: "10.0.0.0/24"}},
			Requests: []RequestInput{
				{ID: "q", PoolID: "nope", Prefix: 32},
			},
		},
		{
			Pools: []PoolInput{
				{ID: "p", CIDR: "10.0.0.0/24"},
				{ID: "p", CIDR: "10.0.1.0/24"},
			},
		},
	}
	for i, task := range cases {
		if _, f := Run(task); f == nil {
			t.Fatalf("case %d: expected failure", i)
		}
	}
}

func TestInputOrderDoesNotMatter(t *testing.T) {
	reqs := []RequestInput{
		{ID: "c", PoolID: "p", Prefix: 32},
		{ID: "a", PoolID: "p", Prefix: 32},
		{ID: "b", PoolID: "p", Prefix: 30},
	}
	t1 := Task{Pools: []PoolInput{{ID: "p", CIDR: "10.0.0.0/24"}}, Requests: reqs}
	t2 := Task{
		Pools:    []PoolInput{{ID: "p", CIDR: "10.0.0.0/24"}},
		Requests: []RequestInput{reqs[2], reqs[0], reqs[1]},
	}
	p1, f1 := Run(t1)
	p2, f2 := Run(t2)
	if f1 != nil || f2 != nil {
		t.Fatalf("unexpected failures %v %v", f1, f2)
	}
	// Allocation proceeds b(/30) first, then a, c; output is sorted by ID.
	want := []string{"a:10.0.0.4/32", "b:10.0.0.0/30", "c:10.0.0.5/32"}
	for i, a := range p1.Allocations {
		got := a.RequestID + ":" + a.CIDR
		if got != want[i] {
			t.Fatalf("p1 alloc[%d] = %s, want %s", i, got, want[i])
		}
		got2 := p2.Allocations[i].RequestID + ":" + p2.Allocations[i].CIDR
		if got2 != want[i] {
			t.Fatalf("p2 alloc[%d] = %s, want %s", i, got2, want[i])
		}
	}
}
