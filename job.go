package chromefleet

import (
	"errors"
	"sync/atomic"
	"time"
)

// JobID identifies a queued unit of work. Assigned by Submit, never reused.
type JobID uint64

var jobSeq uint64

func nextJobID() JobID { return JobID(atomic.AddUint64(&jobSeq, 1)) }

// JobStatus is the terminal disposition of a job after the dispatcher handles
// it. A job is in-flight until one of these values lands on its result chan.
type JobStatus int

const (
	StatusDone      JobStatus = iota // executed successfully
	StatusFailed                     // ran but returned an error
	StatusCancelled                  // aborted before completion (ctx, AbortAll, hotkey)
	StatusRejected                   // rejected at Submit time (validation, fleet stopped)
)

func (s JobStatus) String() string {
	switch s {
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	case StatusRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

// Job is the input to Submit. Priority ties break by insertion order (FIFO).
// Timeout is per-job; zero means use the fleet default.
type Job struct {
	BrowserID string
	Action    Action
	Priority  int
	Timeout   time.Duration
}

// JobResult is the output dispatched to the channel returned by Submit.
type JobResult struct {
	ID        JobID
	BrowserID string
	Status    JobStatus
	Err       error
	Took      time.Duration
}

// queuedJob is the internal record carried through the priority queue. It
// wraps Job with bookkeeping the dispatcher needs (id, insert seq, result chan).
type queuedJob struct {
	id        JobID
	insertSeq uint64
	job       Job
	result    chan JobResult
	enqueued  time.Time
	heapIdx   int // maintained by container/heap
}

// ErrFleetStopped is returned by Submit after Stop or AbortAll.
var ErrFleetStopped = errors.New("chromefleet: fleet stopped, not accepting new jobs")

// ErrUnknownBrowser is returned when Job.BrowserID has not been Register'd.
var ErrUnknownBrowser = errors.New("chromefleet: unknown browser id")
