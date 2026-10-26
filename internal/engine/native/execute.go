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
	files  []*os.File
	buffer []byte
	random *rand.Rand
	offset int64
	cursor int
}

func prepare(ctx context.Context, w domain.ScenarioWorkload, worker int, dir string) (*fileset, error) {
	f := &fileset{buffer: make([]byte, w.Workload.BlockSizeBytes), random: rand.New(rand.NewSource(int64(w.Workload.Seed) + int64(worker)))}
	f.random.Read(f.buffer)
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
	for _, file := range f.files {
		file.Close()
	}
}
func (f *fileset) operation(w domain.ScenarioWorkload) (string, int, error) {
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
	file := f.files[f.cursor%len(f.files)]
	f.cursor++
	if op == "read" {
		n, err := file.ReadAt(f.buffer, offset*int64(len(f.buffer)))
		return op, n, err
	}
	n, err := file.WriteAt(f.buffer, offset*int64(len(f.buffer)))
	return op, n, err
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
					select {
					case <-active.Done():
						return
					case <-queues[i]:
					}
					began := time.Now().UTC()
					if !began.Before(end) {
						return
					}
					op, n, err := f.operation(w)
					finished := time.Now().UTC()
					if !began.Before(start) {
						locks[i].Lock()
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
						locks[i].Unlock()
					}
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
