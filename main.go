package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type WorkerPool struct {
	workers    map[int]*Worker
	taskChan   chan string
	addChan    chan struct{}
	removeChan chan int
	wg         sync.WaitGroup
	workerId   int32
	isRun      bool
	mut        sync.Mutex
	maxWorkers int
}

type Worker struct {
	id       int
	taskChan chan string
	stopChan chan struct{}
}

func newWorker(maxWorkers int) *WorkerPool {
	return &WorkerPool{
		workers:    make(map[int]*Worker),
		taskChan:   make(chan string),
		addChan:    make(chan struct{}),
		removeChan: make(chan int),
		isRun:      true,
		maxWorkers: maxWorkers,
	}
}

func (wp *WorkerPool) Start() {
	go func() {
		for {
			select {
			case <-wp.addChan:
				wp.mut.Lock()
				if wp.isRun && len(wp.workers) < wp.maxWorkers {
					id := int(atomic.AddInt32(&wp.workerId, 1))
					worker := &Worker{
						id:       id,
						taskChan: wp.taskChan,
						stopChan: make(chan struct{}),
					}
					wp.workers[id] = worker
					wp.wg.Add(1)
					go worker.run(&wp.wg)
				}
				wp.mut.Unlock()
			case id := <-wp.removeChan:
				wp.mut.Lock()
				if worker, exists := wp.workers[id]; exists {
					close(worker.stopChan)
					delete(wp.workers, id)
				}
				wp.mut.Unlock()
			}
		}
	}()
}

func (wp *WorkerPool) AddWorker() error {
	wp.mut.Lock()
	if len(wp.workers) >= wp.maxWorkers {
		wp.mut.Unlock()
		return fmt.Errorf("Maximum number of workers achieved")
	}
	wp.mut.Unlock()
	wp.addChan <- struct{}{}

	return nil
}

func (wp *WorkerPool) RemoveWorker(id int) error {
	wp.mut.Lock()
	defer wp.mut.Unlock()
	if worker, exists := wp.workers[id]; exists {
		close(worker.stopChan)
		delete(wp.workers, id)
		return nil
	}
	return fmt.Errorf("worker %d not found", id)
}

func (wp *WorkerPool) Submit(task string) error {
	wp.mut.Lock()
	defer wp.mut.Unlock()
	if !wp.isRun {
		return fmt.Errorf("worker pool stopped")
	}
	select {
	case wp.taskChan <- task:
		return nil
	default:
		return fmt.Errorf("task chan is fulled or closed")
	}
}

func (wp *WorkerPool) Stop() {
	wp.mut.Lock()
	wp.isRun = false

	for _, worker := range wp.workers {
		close(worker.stopChan)
	}

	close(wp.taskChan)
	wp.mut.Unlock()
	wp.wg.Wait()
}

func (w *Worker) run(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case task, ok := <-w.taskChan:
			if !ok {
				fmt.Printf("Worker %d stopped due to closed job channel\n", w.id)
				return
			}
			fmt.Printf("Worker %d processing task:%s\n", w.id, task)
		case <-w.stopChan:
			fmt.Printf("Worker %d is stopped\n", w.id)
			return
		}
	}
}

func main() {
	pool := newWorker(4)
	pool.Start()
	defer pool.Stop()
	pool.AddWorker()
	pool.AddWorker()

	var taskWg sync.WaitGroup
	tasks := []string{"Task 1", "Task 2", "Task 3", "Task 4"}
	for _, task := range tasks {
		taskWg.Add(1)
		go func(t string) {
			defer taskWg.Done()
			if err := pool.Submit(t); err != nil {
				submitErr := fmt.Errorf("Failed to submit task:%v", err)
				fmt.Println(submitErr)
			}
		}(task)
	}
	taskWg.Wait()
	if err := pool.RemoveWorker(1); err != nil {
		removeErr := fmt.Errorf("Failed to remove worker:%v", err)
		fmt.Println(removeErr)
	}
	if err := pool.AddWorker(); err != nil {
		addWorkerError := fmt.Errorf("Failed to add worker: %v", err)
		fmt.Println(addWorkerError)
	}
	taskWg.Add(2)
	for _, task := range []string{"Task 6", "Task 7"} {
		go func(t string) {
			defer taskWg.Done()
			if err := pool.Submit(t); err != nil {
				submitErr := fmt.Errorf("Failed to submit task:%v", err)
				fmt.Println(submitErr)
			}
		}(task)
	}
	taskWg.Wait()
}
