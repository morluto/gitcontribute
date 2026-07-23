package evidence

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

const (
	defaultSampleInterval = 100 * time.Millisecond
	maxSampledProcesses   = 1024
	cleanupGracePeriod    = 500 * time.Millisecond
)

type samplerResult struct {
	identity  ProcessIdentity
	telemetry ResourceTelemetry
	cleanup   CleanupResult
}

type processSampler struct {
	stop chan chan samplerResult
}

func startProcessSampler(ctx context.Context, pid int32, interval time.Duration) *processSampler {
	if interval <= 0 {
		interval = defaultSampleInterval
	}
	s := &processSampler{stop: make(chan chan samplerResult)}
	go s.run(ctx, pid, interval)
	return s
}

func (s *processSampler) finish() samplerResult {
	result := make(chan samplerResult, 1)
	s.stop <- result
	return <-result
}

func (s *processSampler) run(ctx context.Context, pid int32, interval time.Duration) {
	state := newSamplerState(pid, interval)
	state.sample(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			state.sample(ctx)
		case result := <-s.stop:
			result <- state.result(ctx)
			return
		}
	}
}

type samplerState struct {
	rootPID           int32
	rootCreateAt      int64
	interval          time.Duration
	samples           int
	overhead          time.Duration
	peakCPUms         int64
	peakRSS           uint64
	peakChildren      int64
	cpuAvailable      bool
	memoryAvailable   bool
	childrenAvailable bool
	cpuErr            error
	memoryErr         error
	childrenErr       error
	trackingErr       error
	seen              map[ProcessIdentity]struct{}
}

func newSamplerState(pid int32, interval time.Duration) *samplerState {
	return &samplerState{rootPID: pid, interval: interval, seen: make(map[ProcessIdentity]struct{})}
}

func (s *samplerState) sample(parent context.Context) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), min(s.interval, time.Second))
	defer cancel()
	root, err := process.NewProcessWithContext(ctx, s.rootPID)
	if err != nil {
		s.markUnavailable(err)
		s.overhead += time.Since(started)
		return
	}
	createdAt, err := root.CreateTimeWithContext(ctx)
	if err != nil {
		s.markUnavailable(err)
		s.overhead += time.Since(started)
		return
	}
	if s.rootCreateAt == 0 {
		s.rootCreateAt = createdAt
	} else if createdAt != s.rootCreateAt {
		s.markUnavailable(errors.New("root PID was reused"))
		s.overhead += time.Since(started)
		return
	}
	processes, err := processTree(ctx, root)
	if err != nil {
		s.childrenErr = err
		s.trackingErr = err
	}
	var cpuMillis int64
	var rss uint64
	cpuComplete := true
	memoryComplete := true
	for _, current := range processes {
		identity, identityErr := processIdentity(ctx, current)
		if identityErr != nil {
			s.trackingErr = identityErr
		} else {
			s.seen[identity] = struct{}{}
		}
		times, timesErr := current.TimesWithContext(ctx)
		if timesErr != nil {
			s.cpuErr = timesErr
			cpuComplete = false
		} else {
			cpuMillis += int64((times.User + times.System) * 1000)
		}
		memory, memoryErr := current.MemoryInfoWithContext(ctx)
		if memoryErr != nil {
			s.memoryErr = memoryErr
			memoryComplete = false
		} else {
			rss += memory.RSS
		}
	}
	s.samples++
	if cpuComplete {
		s.cpuAvailable = true
		s.peakCPUms = max(s.peakCPUms, cpuMillis)
	}
	if memoryComplete {
		s.memoryAvailable = true
		s.peakRSS = max(s.peakRSS, rss)
	}
	if err == nil {
		s.childrenAvailable = true
		s.peakChildren = max(s.peakChildren, int64(max(0, len(processes)-1)))
	}
	s.overhead += time.Since(started)
}

func (s *samplerState) markUnavailable(err error) {
	if s.cpuErr == nil {
		s.cpuErr = err
	}
	if s.memoryErr == nil {
		s.memoryErr = err
	}
	if s.childrenErr == nil {
		s.childrenErr = err
	}
}

func processTree(ctx context.Context, root *process.Process) ([]*process.Process, error) {
	queue := []*process.Process{root}
	all := make([]*process.Process, 0, 8)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		all = append(all, current)
		if len(all) == maxSampledProcesses {
			return all, fmt.Errorf("process tree exceeds %d entries", maxSampledProcesses)
		}
		children, err := current.ChildrenWithContext(ctx)
		if err != nil {
			return all, err
		}
		queue = append(queue, children...)
	}
	return all, nil
}

func processIdentity(ctx context.Context, item *process.Process) (ProcessIdentity, error) {
	createdAt, err := item.CreateTimeWithContext(ctx)
	if err != nil {
		return ProcessIdentity{}, err
	}
	return ProcessIdentity{PID: item.Pid, CreateTimeUnixMilli: createdAt}, nil
}

func (s *samplerState) result(parent context.Context) samplerResult {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 2*time.Second)
	defer cancel()
	survivors := s.survivors(cleanupCtx)
	deadline := time.Now().Add(cleanupGracePeriod)
	for len(survivors) > 0 && time.Now().Before(deadline) {
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-cleanupCtx.Done():
			timer.Stop()
		case <-timer.C:
		}
		survivors = s.survivors(cleanupCtx)
	}
	sort.Slice(survivors, func(i, j int) bool {
		if survivors[i].PID != survivors[j].PID {
			return survivors[i].PID < survivors[j].PID
		}
		return survivors[i].CreateTimeUnixMilli < survivors[j].CreateTimeUnixMilli
	})
	checkedAt := time.Now().UTC()
	cleanup := CleanupResult{Status: "clean", CheckedAt: checkedAt}
	if len(survivors) > 0 {
		cleanup.Status, cleanup.Reason, cleanup.Survivors = "failed", "sampled descendants survived shutdown", survivors
	}
	telemetry := ResourceTelemetry{
		Provider: "gopsutil/v4", Platform: runtime.GOOS + "/" + runtime.GOARCH,
		SampleInterval: s.interval, SampleCount: s.samples, SamplerOverheadNanoseconds: s.overhead.Nanoseconds(),
		CPUTimeMillis:  metricInt64(s.peakCPUms, unavailableMetricError(s.cpuAvailable, s.cpuErr)),
		PeakRSSBytes:   metricUint64(s.peakRSS, unavailableMetricError(s.memoryAvailable, s.memoryErr)),
		PeakChildCount: metricInt64(s.peakChildren, unavailableMetricError(s.childrenAvailable, s.childrenErr)),
	}
	if s.rootCreateAt == 0 && cleanup.Status == "clean" {
		cleanup.Status, cleanup.Reason = "unavailable", "root process identity was unavailable"
	} else if s.trackingErr != nil && cleanup.Status == "clean" {
		cleanup.Status, cleanup.Reason = "unavailable", "process tree tracking incomplete: "+s.trackingErr.Error()
	}
	return samplerResult{identity: ProcessIdentity{PID: s.rootPID, CreateTimeUnixMilli: s.rootCreateAt}, telemetry: telemetry, cleanup: cleanup}
}

func unavailableMetricError(available bool, lastErr error) error {
	if available {
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("no complete process sample was available")
}

func (s *samplerState) survivors(ctx context.Context) []ProcessIdentity {
	survivors := make([]ProcessIdentity, 0)
	for identity := range s.seen {
		if identity.PID == s.rootPID && identity.CreateTimeUnixMilli == s.rootCreateAt {
			continue
		}
		item, err := process.NewProcessWithContext(ctx, identity.PID)
		if err != nil {
			continue
		}
		createdAt, err := item.CreateTimeWithContext(ctx)
		if err != nil || createdAt != identity.CreateTimeUnixMilli {
			continue
		}
		running, err := item.IsRunningWithContext(ctx)
		if err == nil && running {
			survivors = append(survivors, identity)
		}
	}
	return survivors
}

func metricInt64(value int64, err error) Int64Metric {
	if err != nil {
		return Int64Metric{UnavailableReason: err.Error()}
	}
	return Int64Metric{Value: &value}
}

func metricUint64(value uint64, err error) Uint64Metric {
	if err != nil {
		return Uint64Metric{UnavailableReason: err.Error()}
	}
	return Uint64Metric{Value: &value}
}
