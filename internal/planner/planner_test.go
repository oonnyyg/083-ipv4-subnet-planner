package planner

import (
	"strings"
	"testing"
)

func mustPlan(t *testing.T, task *Task) *Plan {
	t.Helper()
	p, err := PlanTask(task)
	if err != nil {
		t.Fatalf("PlanTask: %v", err)
	}
	return p
}

func mustFail(t *testing.T, task *Task, kind, id string) *ObjectError {
	t.Helper()
	_, err := PlanTask(task)
	if err == nil {
		t.Fatalf("expected failure for %s %q", kind, id)
	}
	oe, ok := err.(*ObjectError)
	if !ok {
		t.Fatalf("expected ObjectError, got %T: %v", err, err)
	}
	if oe.Kind != kind || oe.ID != id {
		t.Fatalf("error points to %s %q, want %s %q", oe.Kind, oe.ID, kind, id)
	}
	return oe
}

func TestDeterministicOrderAndAlignment(t *testing.T) {
	task := &Task{
		Pools: []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}},
		Requests: []Request{
			{ID: "z-big", Pool: "p1", Prefix: 25},
			{ID: "b", Pool: "p1", Prefix: 26},
			{ID: "a", Pool: "p1", Prefix: 26},
		},
	}
	plan := mustPlan(t, task)
	// /26 requests first (smaller prefix number? no: prefix ascending means
	// /25 before /26), then by id.
	want := []struct{ id, cidr string }{
		{"z-big", "10.0.0.0/25"},
		{"a", "10.0.0.128/26"},
		{"b", "10.0.0.192/26"},
	}
	if len(plan.Allocations) != len(want) {
		t.Fatalf("got %d allocations", len(plan.Allocations))
	}
	for i, w := range want {
		if plan.Allocations[i].Request != w.id || plan.Allocations[i].CIDR != w.cidr {
			t.Errorf("alloc[%d] = %s %s, want %s %s",
				i, plan.Allocations[i].Request, plan.Allocations[i].CIDR, w.id, w.cidr)
		}
	}
}

func TestInputOrderDoesNotMatter(t *testing.T) {
	mk := func() *Task {
		return &Task{
			Pools: []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}, {ID: "p2", CIDR: "10.1.0.0/24"}},
			Reserved: []Reserve{
				{ID: "r1", Pool: "p1", CIDR: "10.0.0.0/26"},
				{ID: "r2", Pool: "p2", CIDR: "10.1.0.128/25"},
			},
			Requests: []Request{
				{ID: "a", Pool: "p1", Prefix: 27},
				{ID: "b", Pool: "p2", Prefix: 28},
				{ID: "c", Pool: "p1", Prefix: 27},
			},
		}
	}
	base := mustPlan(t, mk())
	rev := mk()
	rev.Pools[0], rev.Pools[1] = rev.Pools[1], rev.Pools[0]
	rev.Reserved[0], rev.Reserved[1] = rev.Reserved[1], rev.Reserved[0]
	rev.Requests[0], rev.Requests[2] = rev.Requests[2], rev.Requests[0]
	got := mustPlan(t, rev)
	if len(base.Allocations) != len(got.Allocations) {
		t.Fatal("allocation count differs")
	}
	for i := range base.Allocations {
		if base.Allocations[i] != got.Allocations[i] {
			t.Errorf("alloc[%d] differs: %+v vs %+v", i, base.Allocations[i], got.Allocations[i])
		}
	}
}

func TestReservedSkipsAllocation(t *testing.T) {
	task := &Task{
		Pools:    []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}},
		Reserved: []Reserve{{ID: "r1", Pool: "p1", CIDR: "10.0.0.0/25"}},
		Requests: []Request{{ID: "a", Pool: "p1", Prefix: 26}},
	}
	plan := mustPlan(t, task)
	if plan.Allocations[0].CIDR != "10.0.0.128/26" {
		t.Fatalf("got %s", plan.Allocations[0].CIDR)
	}
}

func TestHostBitsRejected(t *testing.T) {
	task := &Task{Pools: []Pool{{ID: "p1", CIDR: "10.0.0.1/24"}}}
	oe := mustFail(t, task, "pool", "p1")
	if !strings.Contains(oe.Msg, "host bits") {
		t.Fatalf("unexpected message: %s", oe.Msg)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name     string
		task     *Task
		kind, id string
	}{
		{"pool overlap", &Task{Pools: []Pool{
			{ID: "p1", CIDR: "10.0.0.0/8"}, {ID: "p2", CIDR: "10.1.0.0/16"}}}, "pool", "p2"},
		{"reserved outside pool", &Task{
			Pools:    []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}},
			Reserved: []Reserve{{ID: "r1", Pool: "p1", CIDR: "10.0.1.0/24"}}}, "reserved", "r1"},
		{"reserved overlap", &Task{
			Pools: []Pool{{ID: "p1", CIDR: "10.0.0.0/8"}},
			Reserved: []Reserve{
				{ID: "r1", Pool: "p1", CIDR: "10.0.0.0/16"},
				{ID: "r2", Pool: "p1", CIDR: "10.0.0.0/24"}}}, "reserved", "r2"},
		{"reserved duplicate", &Task{
			Pools: []Pool{{ID: "p1", CIDR: "10.0.0.0/8"}},
			Reserved: []Reserve{
				{ID: "r1", Pool: "p1", CIDR: "10.0.0.0/16"},
				{ID: "r2", Pool: "p1", CIDR: "10.0.0.0/16"}}}, "reserved", "r2"},
		{"unknown request pool", &Task{
			Pools:    []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}},
			Requests: []Request{{ID: "a", Pool: "nope", Prefix: 24}}}, "request", "a"},
		{"bad prefix", &Task{
			Pools:    []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}},
			Requests: []Request{{ID: "a", Pool: "p1", Prefix: 33}}}, "request", "a"},
		{"duplicate request id", &Task{
			Pools: []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}},
			Requests: []Request{
				{ID: "a", Pool: "p1", Prefix: 24},
				{ID: "a", Pool: "p1", Prefix: 24}}}, "request", "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mustFail(t, c.task, c.kind, c.id)
		})
	}
}

// Fragmented free space: total free is enough but no aligned contiguous
// block fits the request, so the whole plan must fail atomically.
func TestFragmentationFailure(t *testing.T) {
	task := &Task{
		Pools: []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}},
		Reserved: []Reserve{
			{ID: "r1", Pool: "p1", CIDR: "10.0.0.64/26"},
			{ID: "r2", Pool: "p1", CIDR: "10.0.0.192/26"},
		},
		Requests: []Request{{ID: "a", Pool: "p1", Prefix: 25}},
	}
	oe := mustFail(t, task, "request", "a")
	if !strings.Contains(oe.Msg, "no aligned free block") {
		t.Fatalf("unexpected message: %s", oe.Msg)
	}
}

func TestSingleAddressAndWholePool(t *testing.T) {
	task := &Task{
		Pools: []Pool{{ID: "p1", CIDR: "192.168.0.0/24"}},
		Requests: []Request{
			{ID: "host", Pool: "p1", Prefix: 32},
			{ID: "rest", Pool: "p1", Prefix: 24},
		},
	}
	// /24 is processed before /32 (prefix ascending) and consumes the whole
	// pool, so the single-address request must fail.
	mustFail(t, task, "request", "host")

	task.Requests = task.Requests[:1]
	plan := mustPlan(t, task)
	a := plan.Allocations[0]
	if a.CIDR != "192.168.0.0/32" || a.Start != a.End {
		t.Fatalf("got %+v", a)
	}
	if plan.Stats.Allocated != 1 || plan.Stats.Remaining != 255 {
		t.Fatalf("stats: %+v", plan.Stats)
	}
}

// Large pool: full IPv4 space, stats must conserve without per-address work.
func TestLargePoolStatsConservation(t *testing.T) {
	task := &Task{
		Pools:    []Pool{{ID: "all", CIDR: "0.0.0.0/0"}},
		Reserved: []Reserve{{ID: "r1", Pool: "all", CIDR: "10.0.0.0/8"}},
		Requests: []Request{
			{ID: "a", Pool: "all", Prefix: 8},
			{ID: "b", Pool: "all", Prefix: 16},
		},
	}
	plan := mustPlan(t, task)
	s := plan.Stats
	if s.Total != 1<<32 {
		t.Fatalf("total = %d", s.Total)
	}
	if s.Total != s.Reserved+s.Allocated+s.Remaining {
		t.Fatalf("stats not conserved: %+v", s)
	}
	if s.Reserved != 1<<24 || s.Allocated != (1<<24)+(1<<16) {
		t.Fatalf("stats: %+v", s)
	}
	// Smallest aligned /8 free block is 0.0.0.0/8 (reserved is 10.0.0.0/8).
	if plan.Allocations[0].CIDR != "0.0.0.0/8" {
		t.Fatalf("got %s", plan.Allocations[0].CIDR)
	}
}

func TestRemainingMinimalCIDRs(t *testing.T) {
	task := &Task{
		Pools:    []Pool{{ID: "p1", CIDR: "10.0.0.0/24"}},
		Reserved: []Reserve{{ID: "r1", Pool: "p1", CIDR: "10.0.0.128/26"}},
	}
	plan := mustPlan(t, task)
	got := []string{}
	for _, b := range plan.Pools[0].Remaining {
		got = append(got, b.CIDR)
	}
	want := []string{"10.0.0.0/25", "10.0.0.192/26"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("remaining = %v, want %v", got, want)
	}
	if plan.Stats.Remaining != 128+64 {
		t.Fatalf("remaining = %d", plan.Stats.Remaining)
	}
}

func TestNoCrossPoolMerge(t *testing.T) {
	task := &Task{
		Pools: []Pool{
			{ID: "p1", CIDR: "10.0.0.0/25"},
			{ID: "p2", CIDR: "10.0.0.128/25"},
		},
	}
	plan := mustPlan(t, task)
	if len(plan.Pools) != 2 {
		t.Fatalf("got %d pool results", len(plan.Pools))
	}
	for _, pr := range plan.Pools {
		if len(pr.Remaining) != 1 || !strings.HasSuffix(pr.Remaining[0].CIDR, "/25") {
			t.Fatalf("pool %s remaining merged across pools: %+v", pr.ID, pr.Remaining)
		}
	}
}
