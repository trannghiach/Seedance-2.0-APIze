package queue

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/trannghiach/Seedance-2.0-APIze/browser"
	"github.com/trannghiach/Seedance-2.0-APIze/scraper"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

type Job struct {
	ID        string
	Opts      scraper.GenerateOptions
	Status    Status
	VideoPath string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Queue struct {
	mu      sync.RWMutex
	jobs    map[string]*Job
	ch      chan string    // job IDs waiting to be processed
	manager *browser.Manager
	workers int
}

func New(manager *browser.Manager, concurrency int) *Queue {
	q := &Queue{
		jobs:    make(map[string]*Job),
		ch:      make(chan string, 100),
		manager: manager,
		workers: concurrency,
	}

	// Spawn worker goroutines
	// Mỗi worker = 1 browser page = 1 video được xử lý đồng thời
	// Khuyến nghị: concurrency = 1-2 để tránh bị rate limit
	for i := 0; i < concurrency; i++ {
		go q.worker(i)
	}

	return q
}

// Submit thêm job vào queue, trả về job ID ngay lập tức
func (q *Queue) Submit(opts scraper.GenerateOptions) string {
	id := uuid.New().String()

	q.mu.Lock()
	q.jobs[id] = &Job{
		ID:        id,
		Opts:      opts,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	q.mu.Unlock()

	q.ch <- id
	return id
}

// Get trả về trạng thái của job
func (q *Queue) Get(id string) (*Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	job, ok := q.jobs[id]
	return job, ok
}

// worker loop xử lý jobs từ channel
func (q *Queue) worker(id int) {
	fmt.Printf("  Worker %d started\n", id)

	// Mỗi worker có 1 browser page riêng
	page, err := q.manager.NewPage()
	if err != nil {
		fmt.Printf("  Worker %d: failed to create page: %v\n", id, err)
		return
	}

	s := scraper.New(page)

	for jobID := range q.ch {
		q.mu.Lock()
		job := q.jobs[jobID]
		job.Status = StatusProcessing
		job.UpdatedAt = time.Now()
		q.mu.Unlock()

		fmt.Printf("  Worker %d: processing job %s\n", id, jobID[:8])

		result, err := s.Generate(job.Opts)

		q.mu.Lock()
		if err != nil {
			job.Status = StatusFailed
			job.Error = err.Error()
			fmt.Printf("  Worker %d: job %s FAILED: %v\n", id, jobID[:8], err)
		} else {
			job.Status = StatusDone
			job.VideoPath = result.VideoPath
			fmt.Printf("  Worker %d: job %s DONE\n", id, jobID[:8])
		}
		job.UpdatedAt = time.Now()
		q.mu.Unlock()
	}
}