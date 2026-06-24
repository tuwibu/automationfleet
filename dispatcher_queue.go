package automationfleet

import "container/heap"

// priorityQueue orders jobs by Priority desc, then insertSeq asc (FIFO tiebreak).
// Implements heap.Interface; callers use push/pop wrappers below.
type priorityQueue struct {
	items []*queuedJob
}

func (pq priorityQueue) Len() int { return len(pq.items) }

func (pq priorityQueue) Less(i, j int) bool {
	a, b := pq.items[i], pq.items[j]
	if a.job.Priority != b.job.Priority {
		return a.job.Priority > b.job.Priority
	}
	return a.insertSeq < b.insertSeq
}

func (pq priorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.items[i].heapIdx = i
	pq.items[j].heapIdx = j
}

func (pq *priorityQueue) Push(x any) {
	qj := x.(*queuedJob)
	qj.heapIdx = len(pq.items)
	pq.items = append(pq.items, qj)
}

func (pq *priorityQueue) Pop() any {
	n := len(pq.items)
	last := pq.items[n-1]
	pq.items[n-1] = nil
	pq.items = pq.items[:n-1]
	return last
}

// push enqueues qj maintaining heap invariants.
func (pq *priorityQueue) push(qj *queuedJob) { heap.Push(pq, qj) }

// pop removes and returns the highest-priority job, or nil if empty.
func (pq *priorityQueue) pop() *queuedJob {
	if pq.Len() == 0 {
		return nil
	}
	return heap.Pop(pq).(*queuedJob)
}

// drain returns every queued job in arbitrary order, leaving the queue empty.
func (pq *priorityQueue) drain() []*queuedJob {
	out := pq.items
	pq.items = nil
	return out
}
