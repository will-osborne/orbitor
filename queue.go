package main

import "sync"

// PromptQueue serialises prompts so only one is active at a time.
type PromptQueue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	items   []PromptItem
	closed  bool
	onReady func(PromptItem)
}

type PromptItem struct {
	Text string
	Done chan struct{}
}

func NewPromptQueue(onReady func(PromptItem)) *PromptQueue {
	q := &PromptQueue{onReady: onReady}
	q.cond = sync.NewCond(&q.mu)
	go q.run()
	return q
}

func (q *PromptQueue) Enqueue(text string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	item := PromptItem{Text: text, Done: make(chan struct{})}
	q.items = append(q.items, item)
	q.cond.Signal()
}

func (q *PromptQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cond.Signal()
}

func (q *PromptQueue) run() {
	for {
		q.mu.Lock()
		for len(q.items) == 0 && !q.closed {
			q.cond.Wait()
		}
		if q.closed {
			q.mu.Unlock()
			return
		}
		item := q.items[0]
		q.items = q.items[1:]
		q.mu.Unlock()

		q.onReady(item)
		<-item.Done
	}
}
