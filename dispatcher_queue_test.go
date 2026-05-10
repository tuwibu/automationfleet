package chromefleet

import "testing"

func TestPriorityQueueOrder(t *testing.T) {
	pq := &priorityQueue{}

	push := func(seq uint64, prio int) {
		pq.push(&queuedJob{insertSeq: seq, job: Job{Priority: prio}})
	}

	// Mixed priorities, mixed insert order.
	push(1, 5)
	push(2, 9)
	push(3, 5)
	push(4, 1)
	push(5, 9)

	wantSeq := []uint64{2, 5, 1, 3, 4} // prio desc, then FIFO tiebreak
	for i, want := range wantSeq {
		got := pq.pop()
		if got == nil {
			t.Fatalf("pop %d: queue empty", i)
		}
		if got.insertSeq != want {
			t.Errorf("pop %d: want seq %d, got %d (prio %d)", i, want, got.insertSeq, got.job.Priority)
		}
	}
	if pq.pop() != nil {
		t.Errorf("expected empty queue")
	}
}

func TestPriorityQueueDrain(t *testing.T) {
	pq := &priorityQueue{}
	for i := uint64(1); i <= 5; i++ {
		pq.push(&queuedJob{insertSeq: i})
	}
	out := pq.drain()
	if len(out) != 5 {
		t.Fatalf("drain length: want 5, got %d", len(out))
	}
	if pq.Len() != 0 {
		t.Errorf("queue not empty after drain")
	}
}

func TestParseHotkey(t *testing.T) {
	tests := []struct {
		in      string
		wantMod Modifier
		wantKey Key
		wantErr bool
	}{
		{"Ctrl+Alt+Shift+S", ModCtrl | ModAlt | ModShift, KeyS, false},
		{"ctrl+q", ModCtrl, KeyQ, false},
		{"Win+Shift+A", ModWin | ModShift, KeyA, false},
		{"S", 0, 0, true},                // no modifier — reject
		{"Ctrl+", 0, 0, true},             // missing key
		{"Ctrl+5", 0, 0, true},            // digit not supported in this minimal parser
		{"", 0, 0, true},
	}
	for _, tc := range tests {
		got, err := ParseHotkey(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseHotkey(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if got.Mods != tc.wantMod || got.Key != tc.wantKey {
			t.Errorf("ParseHotkey(%q) = {%v, %v}, want {%v, %v}",
				tc.in, got.Mods, got.Key, tc.wantMod, tc.wantKey)
		}
	}
}
