package native

import (
	"context"
	"fmt"
	"github.com/wsq2333/malta/internal/domain"
	"math/bits"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Request struct {
	Plan    domain.Plan `json:"plan"`
	Repeat  int         `json:"repeat"`
	StageID string      `json:"stage_id"`
	DataDir string      `json:"data_dir"`
	OutDir  string      `json:"out_dir"`
}
type fileset struct {
	path       string
	activeFile *os.File
	files      []*os.File
	buffer     []byte
	random     *rand.Rand
	offset     int64
	cursor     int
}

func prepare(ctx context.Context, w domain.ScenarioWorkload, worker int, dir string) (*fileset, error) {
	f := &fileset{buffer: make([]byte, w.Workload.BlockSizeBytes), random: rand.New(rand.NewSource(int64(w.Workload.Seed) + int64(worker)))}
	f.random.Read(f.buffer)
	if w.Kind == "pipeline" {
		f.path = filepath.Join(dir, fmt.Sprintf("worker-%d-cycle", worker))
		return f, nil
	}
	for i := 0; i < w.Workload.FilesPerJob; i++ {
		file, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("worker-%d-file-%d", worker, i)), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			f.close()
			return nil, err
		}
		f.files = append(f.files, file)
		for offset := int64(0); offset < w.Workload.FileSizeBytes; offset += int64(len(f.buffer)) {
			if err = ctx.Err(); err != nil {
				f.close()
				return nil, err
			}
			if _, err = file.Write(f.buffer); err != nil {
				f.close()
				return nil, err
			}
		}
		if err = file.Sync(); err != nil {
			f.close()
			return nil, err
		}
	}
	return f, nil
}
func (f *fileset) close() {
	if f.activeFile != nil {
		f.activeFile.Close()
	}
	for _, file := range f.files {
		file.Close()
	}
}

type observer func(string, int, error, time.Time, time.Time)

func (f *fileset) perform(w domain.ScenarioWorkload, record observer) error {
	if w.Kind == "pipeline" {
		for _, op := range w.Steps {
			var n int
			var err error
			began := time.Now().UTC()
			switch op {
			case "create":
				f.activeFile, err = os.OpenFile(f.path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
			case "write":
				n, err = f.activeFile.WriteAt(f.buffer, 0)
			case "fsync":
				err = f.activeFile.Sync()
			case "stat":
				_, err = os.Stat(f.path)
			case "read":
				n, err = f.activeFile.ReadAt(f.buffer, 0)
			case "unlink":
				err = os.Remove(f.path)
			}
			finished := time.Now().UTC()
			record(op, n, err, began, finished)
			if err != nil {
				return err
			}
			if op == "unlink" {
				if err = f.activeFile.Close(); err != nil {
					return err
				}
				f.activeFile = nil
			}
		}
		return nil
	}
	file := f.files[f.cursor%len(f.files)]
	f.cursor++
	if w.Kind == "metadata" {
		began := time.Now().UTC()
		_, err := os.Stat(file.Name())
		record("stat", 0, err, began, time.Now().UTC())
		return err
	}
	op := w.Workload.Operation
	random := op == "randread" || op == "randwrite" || op == "randrw"
	if op == "randrw" {
		if f.random.Intn(100) < w.Workload.ReadPercent {
			op = "read"
		} else {
			op = "write"
		}
	} else if op == "randread" {
		op = "read"
	} else if op == "randwrite" {
		op = "write"
	}
	blocks := w.Workload.FileSizeBytes / int64(len(f.buffer))
	offset := f.offset % blocks
	if random {
		offset = f.random.Int63n(blocks)
	}
	f.offset++
	began := time.Now().UTC()
	var n int
	var err error
	if op == "read" {
		n, err = file.ReadAt(f.buffer, offset*int64(len(f.buffer)))
	} else {
		n, err = file.WriteAt(f.buffer, offset*int64(len(f.buffer)))
	}
	record(op, n, err, began, time.Now().UTC())
	return err
}

// Smooth weighted round-robin determines submission order; waiting for a free worker
// in the selected workload applies backpressure instead of silently changing the mix.
type Scheduler struct {
	weights, current []int
	order            []int
	total            int
}

func NewScheduler(weights []int, seed int64) *Scheduler {
	s := &Scheduler{weights: weights, current: make([]int, len(weights)), order: rand.New(rand.NewSource(seed)).Perm(len(weights))}
	for _, w := range weights {
		s.total += w
	}
	return s
}
func (s *Scheduler) Next() int {
	chosen := -1
	for _, i := range s.order {
		s.current[i] += s.weights[i]
		if chosen < 0 || s.current[i] > s.current[chosen] {
			chosen = i
		}
	}
	s.current[chosen] -= s.total
	return chosen
}
func Execute(ctx context.Context, q Request) error {
	if err := q.Plan.Validate(); err != nil {
		return err
	}
	if q.Plan.Spec.Scenario == nil {
		return domain.E("invalid", "native worker requires scenario")
	}
	var stage domain.Stage
	for _, s := range q.Plan.Spec.Scenario.Stages {
		if s.ID == q.StageID {
			stage = s
		}
	}
	if !stage.Native() || q.Repeat < 1 || q.Repeat > q.Plan.Spec.Repeats {
		return domain.E("invalid", "invalid native stage or repeat")
	}
	count := len(stage.Workloads)
	sets := make([][]*fileset, count)
	points := make([]domain.Point, count)
	ev := make([]Evidence, count)
	defer func() {
		for _, workers := range sets {
			for _, f := range workers {
				f.close()
			}
		}
	}()
	for i, w := range stage.Workloads {
		for _, p := range q.Plan.Trials {
			if p.StageID == stage.ID && p.WorkloadID == w.ID && p.Repeat == q.Repeat {
				points[i] = p
			}
		}
		dir := filepath.Join(q.DataDir, points[i].ID)
		if err := os.Mkdir(dir, 0700); err != nil {
			return err
		}
		if err := os.Mkdir(filepath.Join(q.OutDir, points[i].ID), 0755); err != nil {
			return err
		}
		for worker := 0; worker < w.NumJobs; worker++ {
			f, err := prepare(ctx, w, worker, dir)
			if err != nil {
				return err
			}
			sets[i] = append(sets[i], f)
		}
	}
	start := time.Now().UTC().Add(time.Duration(stage.WarmupSeconds) * time.Second)
	end := start.Add(time.Duration(stage.RuntimeSeconds) * time.Second)
	active, cancel := context.WithDeadline(ctx, end)
	defer cancel()
	queues := make([]chan struct{}, count)
	locks := make([]sync.Mutex, count)
	failures := make(chan error, 1)
	var wg sync.WaitGroup
	for i, w := range stage.Workloads {
		queues[i] = make(chan struct{})
		ev[i] = Evidence{Version: Version, StageHash: domain.Hash(stage), Point: points[i], WindowStart: start, WindowEnd: end, Counters: []Counter{}}
		for _, op := range domain.NativeOperations(w) {
			ev[i].Counters = append(ev[i].Counters, Counter{Operation: op})
		}
		for _, f := range sets[i] {
			wg.Add(1)
			go func(i int, w domain.ScenarioWorkload, f *fileset) {
				defer wg.Done()
				for {
					if stage.Mode == "weighted" {
						select {
						case <-active.Done():
							return
						case <-queues[i]:
						}
					} else if active.Err() != nil {
						return
					}
					if !time.Now().Before(end) {
						return
					}
					err := f.perform(w, func(op string, n int, err error, began, finished time.Time) {
						if began.Before(start) || !began.Before(end) {
							return
						}
						locks[i].Lock()
						defer locks[i].Unlock()
						for j := range ev[i].Counters {
							c := &ev[i].Counters[j]
							if c.Operation != op {
								continue
							}
							c.Submitted++
							if err != nil {
								c.Errors++
							} else {
								c.Succeeded++
								if finished.After(end) {
									c.Late++
								} else {
									c.Bytes += uint64(n)
									c.Histogram[bits.Len64(uint64(finished.Sub(began).Nanoseconds()))]++
									if ev[i].FirstIO.IsZero() || began.Before(ev[i].FirstIO) {
										ev[i].FirstIO = began
									}
									if finished.After(ev[i].LastIO) {
										ev[i].LastIO = finished
									}
								}
							}
							break
						}
					})
					if err != nil {
						select {
						case failures <- err:
						default:
						}
						cancel()
						return
					}
				}
			}(i, w, f)
		}
	}
	if stage.Mode == "weighted" {
		weights := make([]int, count)
		for i, w := range stage.Workloads {
			weights[i] = stage.Mix.Weights[w.ID]
		}
		sched := NewScheduler(weights, stage.Mix.Seed)
	dispatch:
		for {
			next := sched.Next()
			select {
			case <-active.Done():
				break dispatch
			case queues[next] <- struct{}{}:
			}
		}
	} else {
		<-active.Done()
	}

	cancel()
	wg.Wait()
	for i := range ev {
		if err := domain.Write(filepath.Join(q.OutDir, points[i].ID, "native.json"), ev[i]); err != nil {
			return err
		}
	}
	select {
	case err := <-failures:
		return err
	default:
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}
