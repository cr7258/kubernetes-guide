/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package queueset

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apiserver/pkg/util/flowcontrol/counter"
	fq "k8s.io/apiserver/pkg/util/flowcontrol/fairqueuing"
	test "k8s.io/apiserver/pkg/util/flowcontrol/fairqueuing/testing"
	testclock "k8s.io/apiserver/pkg/util/flowcontrol/fairqueuing/testing/clock"
	"k8s.io/apiserver/pkg/util/flowcontrol/metrics"
	fcrequest "k8s.io/apiserver/pkg/util/flowcontrol/request"
	"k8s.io/klog/v2"
)

// fairAlloc computes the max-min fair allocation of the given
// capacity to the given demands (which slice is not side-effected).
func fairAlloc(demands []float64, capacity float64) []float64 {
	count := len(demands)
	indices := make([]int, count)
	for i := 0; i < count; i++ {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool { return demands[indices[i]] < demands[indices[j]] })
	alloc := make([]float64, count)
	var next int
	var prevAlloc float64
	for ; next < count; next++ {
		// `capacity` is how much remains assuming that
		// all unvisited items get `prevAlloc`.
		idx := indices[next]
		demand := demands[idx]
		if demand <= 0 {
			continue
		}
		// `fullCapacityBite` is how much more capacity would be used
		// if this and all following items get as much as this one
		// is demanding.
		fullCapacityBite := float64(count-next) * (demand - prevAlloc)
		if fullCapacityBite > capacity {
			break
		}
		prevAlloc = demand
		alloc[idx] = demand
		capacity -= fullCapacityBite
	}
	for j := next; j < count; j++ {
		alloc[indices[j]] = prevAlloc + capacity/float64(count-next)
	}
	return alloc
}

func TestFairAlloc(t *testing.T) {
	if e, a := []float64{0, 0}, fairAlloc([]float64{0, 0}, 42); !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %#+v, got #%+v", e, a)
	}
	if e, a := []float64{42, 0}, fairAlloc([]float64{47, 0}, 42); !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %#+v, got #%+v", e, a)
	}
	if e, a := []float64{1, 41}, fairAlloc([]float64{1, 47}, 42); !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %#+v, got #%+v", e, a)
	}
	if e, a := []float64{3, 5, 5, 1}, fairAlloc([]float64{3, 7, 9, 1}, 14); !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %#+v, got #%+v", e, a)
	}
	if e, a := []float64{1, 9, 7, 3}, fairAlloc([]float64{1, 9, 7, 3}, 21); !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %#+v, got #%+v", e, a)
	}
}

type uniformClient struct {
	hash     uint64
	nThreads int
	nCalls   int
	// duration for a simulated synchronous call
	execDuration time.Duration
	// duration for simulated "other work".  This can be negative,
	// causing a request to be launched a certain amount of time
	// before the previous one finishes.
	thinkDuration time.Duration
	// When true indicates that only half the specified number of
	// threads should run during the first half of the evaluation
	// period
	split bool
}

// uniformScenario describes a scenario based on the given set of uniform clients.
// Each uniform client specifies a number of threads, each of which alternates between thinking
// and making a synchronous request through the QueueSet.
// The test measures how much concurrency each client got, on average, over
// the initial evalDuration and tests to see whether they all got about the fair amount.
// Each client needs to be demanding enough to use more than its fair share,
// or overall care needs to be taken about timing so that scheduling details
// do not cause any client to actually request a significantly smaller share
// than it theoretically should.
// expectFair indicate whether the QueueSet is expected to be
// fair in the respective halves of a split scenario;
// in a non-split scenario this is a singleton with one expectation.
// expectAllRequests indicates whether all requests are expected to get dispatched.
type uniformScenario struct {
	name                                     string
	qs                                       fq.QueueSet
	clients                                  []uniformClient
	concurrencyLimit                         int
	evalDuration                             time.Duration
	expectedFair                             []bool
	expectedFairnessMargin                   []float64
	expectAllRequests                        bool
	evalInqueueMetrics, evalExecutingMetrics bool
	rejectReason                             string
	clk                                      *testclock.FakeEventClock
	counter                                  counter.GoRoutineCounter
}

func (us uniformScenario) exercise(t *testing.T) {
	uss := uniformScenarioState{
		t:               t,
		uniformScenario: us,
		startTime:       time.Now(),
		integrators:     make([]fq.Integrator, len(us.clients)),
		executions:      make([]int32, len(us.clients)),
		rejects:         make([]int32, len(us.clients)),
	}
	for _, uc := range us.clients {
		uss.doSplit = uss.doSplit || uc.split
	}
	uss.exercise()
}

type uniformScenarioState struct {
	t *testing.T
	uniformScenario
	startTime                                                    time.Time
	doSplit                                                      bool
	integrators                                                  []fq.Integrator
	failedCount                                                  uint64
	expectedInqueue, expectedExecuting, expectedConcurrencyInUse string
	executions, rejects                                          []int32
}

func (uss *uniformScenarioState) exercise() {
	uss.t.Logf("%s: Start %s, doSplit=%v, clk=%p, grc=%p", uss.startTime.Format(nsTimeFmt), uss.name, uss.doSplit, uss.clk, uss.counter)
	if uss.evalInqueueMetrics || uss.evalExecutingMetrics {
		metrics.Reset()
	}
	for i, uc := range uss.clients {
		uss.integrators[i] = fq.NewIntegrator(uss.clk)
		fsName := fmt.Sprintf("client%d", i)
		uss.expectedInqueue = uss.expectedInqueue + fmt.Sprintf(`				apiserver_flowcontrol_current_inqueue_requests{flow_schema=%q,priority_level=%q} 0%s`, fsName, uss.name, "\n")
		for j := 0; j < uc.nThreads; j++ {
			ust := uniformScenarioThread{
				uss:    uss,
				i:      i,
				j:      j,
				nCalls: uc.nCalls,
				uc:     uc,
				igr:    uss.integrators[i],
				fsName: fsName,
			}
			ust.start()
		}
	}
	if uss.doSplit {
		uss.evalTo(uss.startTime.Add(uss.evalDuration/2), false, uss.expectedFair[0], uss.expectedFairnessMargin[0])
	}
	uss.evalTo(uss.startTime.Add(uss.evalDuration), true, uss.expectedFair[len(uss.expectedFair)-1], uss.expectedFairnessMargin[len(uss.expectedFairnessMargin)-1])
	uss.clk.Run(nil)
	uss.finalReview()
}

type uniformScenarioThread struct {
	uss    *uniformScenarioState
	i, j   int
	nCalls int
	uc     uniformClient
	igr    fq.Integrator
	fsName string
}

func (ust *uniformScenarioThread) start() {
	initialDelay := time.Duration(11*ust.j + 2*ust.i)
	if ust.uc.split && ust.j >= ust.uc.nThreads/2 {
		initialDelay += ust.uss.evalDuration / 2
		ust.nCalls = ust.nCalls / 2
	}
	ust.uss.clk.EventAfterDuration(ust.genCallK(0), initialDelay)
}

// generates an EventFunc that forks a goroutine to do call k
func (ust *uniformScenarioThread) genCallK(k int) func(time.Time) {
	return func(time.Time) {
		// As an EventFunc, this has to return without waiting
		// for time to pass, and so cannot do callK(k) itself.
		ust.uss.counter.Add(1)
		go func() {
			ust.callK(k)
			ust.uss.counter.Add(-1)
		}()
	}
}

func (ust *uniformScenarioThread) callK(k int) {
	if k >= ust.nCalls {
		return
	}
	req, idle := ust.uss.qs.StartRequest(context.Background(), &fcrequest.Width{Seats: 1}, ust.uc.hash, "", ust.fsName, ust.uss.name, []int{ust.i, ust.j, k}, nil)
	ust.uss.t.Logf("%s: %d, %d, %d got req=%p, idle=%v", ust.uss.clk.Now().Format(nsTimeFmt), ust.i, ust.j, k, req, idle)
	if req == nil {
		atomic.AddUint64(&ust.uss.failedCount, 1)
		atomic.AddInt32(&ust.uss.rejects[ust.i], 1)
		return
	}
	if idle {
		ust.uss.t.Error("got request but QueueSet reported idle")
	}
	var executed bool
	idle2 := req.Finish(func() {
		executed = true
		execStart := ust.uss.clk.Now()
		ust.uss.t.Logf("%s: %d, %d, %d executing", execStart.Format(nsTimeFmt), ust.i, ust.j, k)
		atomic.AddInt32(&ust.uss.executions[ust.i], 1)
		ust.igr.Add(1)
		ust.uss.clk.EventAfterDuration(ust.genCallK(k+1), ust.uc.execDuration+ust.uc.thinkDuration)
		ClockWait(ust.uss.clk, ust.uss.counter, ust.uc.execDuration)
		ust.igr.Add(-1)
	})
	ust.uss.t.Logf("%s: %d, %d, %d got executed=%v, idle2=%v", ust.uss.clk.Now().Format(nsTimeFmt), ust.i, ust.j, k, executed, idle2)
	if !executed {
		atomic.AddUint64(&ust.uss.failedCount, 1)
		atomic.AddInt32(&ust.uss.rejects[ust.i], 1)
	}
}

func (uss *uniformScenarioState) evalTo(lim time.Time, last, expectFair bool, margin float64) {
	uss.clk.Run(&lim)
	uss.clk.SetTime(lim)
	if uss.doSplit && !last {
		uss.t.Logf("%s: End of first half", uss.clk.Now().Format(nsTimeFmt))
	} else {
		uss.t.Logf("%s: End", uss.clk.Now().Format(nsTimeFmt))
	}
	demands := make([]float64, len(uss.clients))
	averages := make([]float64, len(uss.clients))
	for i, uc := range uss.clients {
		nThreads := uc.nThreads
		if uc.split && !last {
			nThreads = nThreads / 2
		}
		demands[i] = float64(nThreads) * float64(uc.execDuration) / float64(uc.thinkDuration+uc.execDuration)
		averages[i] = uss.integrators[i].Reset().Average
	}
	fairAverages := fairAlloc(demands, float64(uss.concurrencyLimit))
	for i := range uss.clients {
		var gotFair bool
		if fairAverages[i] > 0 {
			relDiff := (averages[i] - fairAverages[i]) / fairAverages[i]
			gotFair = math.Abs(relDiff) <= margin
		} else {
			gotFair = math.Abs(averages[i]) <= margin
		}

		if gotFair != expectFair {
			uss.t.Errorf("%s client %d last=%v got an Average of %v but the fair average was %v", uss.name, i, last, averages[i], fairAverages[i])
		} else {
			uss.t.Logf("%s client %d last=%v got an Average of %v and the fair average was %v", uss.name, i, last, averages[i], fairAverages[i])
		}
	}
}

func (uss *uniformScenarioState) finalReview() {
	if uss.expectAllRequests && uss.failedCount > 0 {
		uss.t.Errorf("Expected all requests to be successful but got %v failed requests", uss.failedCount)
	} else if !uss.expectAllRequests && uss.failedCount == 0 {
		uss.t.Errorf("Expected failed requests but all requests succeeded")
	}
	if uss.evalInqueueMetrics {
		e := `
				# HELP apiserver_flowcontrol_current_inqueue_requests [ALPHA] Number of requests currently pending in queues of the API Priority and Fairness system
				# TYPE apiserver_flowcontrol_current_inqueue_requests gauge
` + uss.expectedInqueue
		err := metrics.GatherAndCompare(e, "apiserver_flowcontrol_current_inqueue_requests")
		if err != nil {
			uss.t.Error(err)
		} else {
			uss.t.Log("Success with" + e)
		}
	}
	expectedRejects := ""
	for i := range uss.clients {
		fsName := fmt.Sprintf("client%d", i)
		if atomic.AddInt32(&uss.executions[i], 0) > 0 {
			uss.expectedExecuting = uss.expectedExecuting + fmt.Sprintf(`				apiserver_flowcontrol_current_executing_requests{flow_schema=%q,priority_level=%q} 0%s`, fsName, uss.name, "\n")
			uss.expectedConcurrencyInUse = uss.expectedConcurrencyInUse + fmt.Sprintf(`				apiserver_flowcontrol_request_concurrency_in_use{flow_schema=%q,priority_level=%q} 0%s`, fsName, uss.name, "\n")
		}
		if atomic.AddInt32(&uss.rejects[i], 0) > 0 {
			expectedRejects = expectedRejects + fmt.Sprintf(`				apiserver_flowcontrol_rejected_requests_total{flow_schema=%q,priority_level=%q,reason=%q} %d%s`, fsName, uss.name, uss.rejectReason, uss.rejects[i], "\n")
		}
	}
	if uss.evalExecutingMetrics && len(uss.expectedExecuting) > 0 {
		e := `
				# HELP apiserver_flowcontrol_current_executing_requests [ALPHA] Number of requests currently executing in the API Priority and Fairness system
				# TYPE apiserver_flowcontrol_current_executing_requests gauge
` + uss.expectedExecuting
		err := metrics.GatherAndCompare(e, "apiserver_flowcontrol_current_executing_requests")
		if err != nil {
			uss.t.Error(err)
		} else {
			uss.t.Log("Success with" + e)
		}
	}
	if uss.evalExecutingMetrics && len(uss.expectedConcurrencyInUse) > 0 {
		e := `
				# HELP apiserver_flowcontrol_request_concurrency_in_use [ALPHA] Concurrency (number of seats) occupided by the currently executing requests in the API Priority and Fairness system
				# TYPE apiserver_flowcontrol_request_concurrency_in_use gauge
` + uss.expectedConcurrencyInUse
		err := metrics.GatherAndCompare(e, "apiserver_flowcontrol_request_concurrency_in_use")
		if err != nil {
			uss.t.Error(err)
		} else {
			uss.t.Log("Success with" + e)
		}
	}
	if uss.evalExecutingMetrics && len(expectedRejects) > 0 {
		e := `
				# HELP apiserver_flowcontrol_rejected_requests_total [ALPHA] Number of requests rejected by API Priority and Fairness system
				# TYPE apiserver_flowcontrol_rejected_requests_total counter
` + expectedRejects
		err := metrics.GatherAndCompare(e, "apiserver_flowcontrol_rejected_requests_total")
		if err != nil {
			uss.t.Error(err)
		} else {
			uss.t.Log("Success with" + e)
		}
	}
}

func ClockWait(clk *testclock.FakeEventClock, counter counter.GoRoutineCounter, duration time.Duration) {
	dunch := make(chan struct{})
	clk.EventAfterDuration(func(time.Time) {
		counter.Add(1)
		close(dunch)
	}, duration)
	counter.Add(-1)
	<-dunch
}

func init() {
	klog.InitFlags(nil)
}

// TestNoRestraint tests whether the no-restraint factory gives every client what it asks for
func TestNoRestraint(t *testing.T) {
	metrics.Register()
	now := time.Now()
	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	nrc, err := test.NewNoRestraintFactory().BeginConstruction(fq.QueuingConfig{}, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	nr := nrc.Complete(fq.DispatchingConfig{})
	uniformScenario{name: "NoRestraint",
		qs: nr,
		clients: []uniformClient{
			{1001001001, 5, 10, time.Second, time.Second, false},
			{2002002002, 2, 10, time.Second, time.Second / 2, false},
		},
		concurrencyLimit:       10,
		evalDuration:           time.Second * 15,
		expectedFair:           []bool{true},
		expectedFairnessMargin: []float64{0.1},
		expectAllRequests:      true,
		clk:                    clk,
		counter:                counter,
	}.exercise(t)
}

func TestUniformFlowsHandSize1(t *testing.T) {
	metrics.Register()
	now := time.Now()

	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "TestUniformFlowsHandSize1",
		DesiredNumQueues: 9,
		QueueLengthLimit: 8,
		HandSize:         1,
		RequestWaitLimit: 10 * time.Minute,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 4})

	uniformScenario{name: qCfg.Name,
		qs: qs,
		clients: []uniformClient{
			{1001001001, 8, 20, time.Second, time.Second - 1, false},
			{2002002002, 8, 20, time.Second, time.Second - 1, false},
		},
		concurrencyLimit:       4,
		evalDuration:           time.Second * 50,
		expectedFair:           []bool{true},
		expectedFairnessMargin: []float64{0.1},
		expectAllRequests:      true,
		evalInqueueMetrics:     true,
		evalExecutingMetrics:   true,
		clk:                    clk,
		counter:                counter,
	}.exercise(t)
}

func TestUniformFlowsHandSize3(t *testing.T) {
	metrics.Register()
	now := time.Now()

	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "TestUniformFlowsHandSize3",
		DesiredNumQueues: 8,
		QueueLengthLimit: 4,
		HandSize:         3,
		RequestWaitLimit: 10 * time.Minute,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 4})
	uniformScenario{name: qCfg.Name,
		qs: qs,
		clients: []uniformClient{
			{1001001001, 8, 30, time.Second, time.Second - 1, false},
			{2002002002, 8, 30, time.Second, time.Second - 1, false},
		},
		concurrencyLimit:       4,
		evalDuration:           time.Second * 60,
		expectedFair:           []bool{true},
		expectedFairnessMargin: []float64{0.1},
		expectAllRequests:      true,
		evalInqueueMetrics:     true,
		evalExecutingMetrics:   true,
		clk:                    clk,
		counter:                counter,
	}.exercise(t)
}

func TestDifferentFlowsExpectEqual(t *testing.T) {
	metrics.Register()
	now := time.Now()

	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "DiffFlowsExpectEqual",
		DesiredNumQueues: 9,
		QueueLengthLimit: 8,
		HandSize:         1,
		RequestWaitLimit: 10 * time.Minute,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 4})

	uniformScenario{name: qCfg.Name,
		qs: qs,
		clients: []uniformClient{
			{1001001001, 8, 20, time.Second, time.Second, false},
			{2002002002, 7, 30, time.Second, time.Second / 2, false},
		},
		concurrencyLimit:       4,
		evalDuration:           time.Second * 40,
		expectedFair:           []bool{true},
		expectedFairnessMargin: []float64{0.1},
		expectAllRequests:      true,
		evalInqueueMetrics:     true,
		evalExecutingMetrics:   true,
		clk:                    clk,
		counter:                counter,
	}.exercise(t)
}

func TestDifferentFlowsExpectUnequal(t *testing.T) {
	metrics.Register()
	now := time.Now()

	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "DiffFlowsExpectUnequal",
		DesiredNumQueues: 9,
		QueueLengthLimit: 6,
		HandSize:         1,
		RequestWaitLimit: 10 * time.Minute,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 3})

	uniformScenario{name: qCfg.Name,
		qs: qs,
		clients: []uniformClient{
			{1001001001, 4, 20, time.Second, time.Second - 1, false},
			{2002002002, 2, 20, time.Second, time.Second - 1, false},
		},
		concurrencyLimit:       3,
		evalDuration:           time.Second * 20,
		expectedFair:           []bool{true},
		expectedFairnessMargin: []float64{0.1},
		expectAllRequests:      true,
		evalInqueueMetrics:     true,
		evalExecutingMetrics:   true,
		clk:                    clk,
		counter:                counter,
	}.exercise(t)
}

func TestWindup(t *testing.T) {
	metrics.Register()
	now := time.Now()

	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "TestWindup",
		DesiredNumQueues: 9,
		QueueLengthLimit: 6,
		HandSize:         1,
		RequestWaitLimit: 10 * time.Minute,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 3})

	uniformScenario{name: qCfg.Name, qs: qs,
		clients: []uniformClient{
			{1001001001, 2, 40, time.Second, -1, false},
			{2002002002, 2, 40, time.Second, -1, true},
		},
		concurrencyLimit:       3,
		evalDuration:           time.Second * 40,
		expectedFair:           []bool{true, true},
		expectedFairnessMargin: []float64{0.1, 0.26},
		expectAllRequests:      true,
		evalInqueueMetrics:     true,
		evalExecutingMetrics:   true,
		clk:                    clk,
		counter:                counter,
	}.exercise(t)
}

func TestDifferentFlowsWithoutQueuing(t *testing.T) {
	metrics.Register()
	now := time.Now()

	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "TestDifferentFlowsWithoutQueuing",
		DesiredNumQueues: 0,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 4})

	uniformScenario{name: qCfg.Name,
		qs: qs,
		clients: []uniformClient{
			{1001001001, 6, 10, time.Second, 57 * time.Millisecond, false},
			{2002002002, 4, 15, time.Second, 750 * time.Millisecond, false},
		},
		concurrencyLimit:       4,
		evalDuration:           time.Second * 13,
		expectedFair:           []bool{false},
		expectedFairnessMargin: []float64{0.1},
		evalExecutingMetrics:   true,
		rejectReason:           "concurrency-limit",
		clk:                    clk,
		counter:                counter,
	}.exercise(t)
}

func TestTimeout(t *testing.T) {
	metrics.Register()
	now := time.Now()

	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "TestTimeout",
		DesiredNumQueues: 128,
		QueueLengthLimit: 128,
		HandSize:         1,
		RequestWaitLimit: 0,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 1})

	uniformScenario{name: qCfg.Name,
		qs: qs,
		clients: []uniformClient{
			{1001001001, 5, 100, time.Second, time.Second, false},
		},
		concurrencyLimit:       1,
		evalDuration:           time.Second * 10,
		expectedFair:           []bool{true},
		expectedFairnessMargin: []float64{0.1},
		evalInqueueMetrics:     true,
		evalExecutingMetrics:   true,
		rejectReason:           "time-out",
		clk:                    clk,
		counter:                counter,
	}.exercise(t)
}

func TestContextCancel(t *testing.T) {
	metrics.Register()
	metrics.Reset()
	now := time.Now()
	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "TestContextCancel",
		DesiredNumQueues: 11,
		QueueLengthLimit: 11,
		HandSize:         1,
		RequestWaitLimit: 15 * time.Second,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 1})
	counter.Add(1) // account for the goroutine running this test
	ctx1 := context.Background()
	b2i := map[bool]int{false: 0, true: 1}
	var qnc [2][2]int32
	req1, _ := qs.StartRequest(ctx1, &fcrequest.Width{Seats: 1}, 1, "", "fs1", "test", "one", func(inQueue bool) { atomic.AddInt32(&qnc[0][b2i[inQueue]], 1) })
	if req1 == nil {
		t.Error("Request rejected")
		return
	}
	if a := atomic.AddInt32(&qnc[0][0], 0); a != 1 {
		t.Errorf("Got %d calls to queueNoteFn1(false), expected 1", a)
	}
	if a := atomic.AddInt32(&qnc[0][1], 0); a != 1 {
		t.Errorf("Got %d calls to queueNoteFn1(true), expected 1", a)
	}
	var executed1 bool
	idle1 := req1.Finish(func() {
		executed1 = true
		ctx2, cancel2 := context.WithCancel(context.Background())
		tBefore := time.Now()
		go func() {
			time.Sleep(time.Second)
			if a := atomic.AddInt32(&qnc[1][0], 0); a != 0 {
				t.Errorf("Got %d calls to queueNoteFn2(false), expected 0", a)
			}
			if a := atomic.AddInt32(&qnc[1][1], 0); a != 1 {
				t.Errorf("Got %d calls to queueNoteFn2(true), expected 1", a)
			}
			// account for unblocking the goroutine that waits on cancelation
			counter.Add(1)
			cancel2()
		}()
		req2, idle2a := qs.StartRequest(ctx2, &fcrequest.Width{Seats: 1}, 2, "", "fs2", "test", "two", func(inQueue bool) { atomic.AddInt32(&qnc[1][b2i[inQueue]], 1) })
		if idle2a {
			t.Error("2nd StartRequest returned idle")
		}
		if req2 != nil {
			idle2b := req2.Finish(func() {
				t.Error("Executing req2")
			})
			if idle2b {
				t.Error("2nd Finish returned idle")
			}
			if a := atomic.AddInt32(&qnc[1][0], 0); a != 1 {
				t.Errorf("Got %d calls to queueNoteFn2(false), expected 1", a)
			}
		}
		tAfter := time.Now()
		dt := tAfter.Sub(tBefore)
		if dt < time.Second || dt > 2*time.Second {
			t.Errorf("Unexpected: dt=%d", dt)
		}
	})
	if !executed1 {
		t.Errorf("Unexpected: executed1=%v", executed1)
	}
	if !idle1 {
		t.Error("Not idle at the end")
	}
}

func TestTotalRequestsExecutingWithPanic(t *testing.T) {
	metrics.Register()
	metrics.Reset()
	now := time.Now()
	clk, counter := testclock.NewFakeEventClock(now, 0, nil)
	qsf := NewQueueSetFactory(clk, counter)
	qCfg := fq.QueuingConfig{
		Name:             "TestTotalRequestsExecutingWithPanic",
		DesiredNumQueues: 0,
		RequestWaitLimit: 15 * time.Second,
	}
	qsc, err := qsf.BeginConstruction(qCfg, newObserverPair(clk))
	if err != nil {
		t.Fatal(err)
	}
	qs := qsc.Complete(fq.DispatchingConfig{ConcurrencyLimit: 1})
	counter.Add(1) // account for the goroutine running this test

	queue, ok := qs.(*queueSet)
	if !ok {
		t.Fatalf("expected a QueueSet of type: %T but got: %T", &queueSet{}, qs)
	}
	if queue.totRequestsExecuting != 0 {
		t.Fatalf("precondition: expected total requests currently executing of the QueueSet to be 0, but got: %d", queue.totRequestsExecuting)
	}
	if queue.dCfg.ConcurrencyLimit != 1 {
		t.Fatalf("precondition: expected concurrency limit of the QueueSet to be 1, but got: %d", queue.dCfg.ConcurrencyLimit)
	}

	ctx := context.Background()
	req, _ := qs.StartRequest(ctx, &fcrequest.Width{Seats: 1}, 1, "", "fs", "test", "one", func(inQueue bool) {})
	if req == nil {
		t.Fatal("expected a Request object from StartRequest, but got nil")
	}

	panicErrExpected := errors.New("apiserver panic'd")
	var panicErrGot interface{}
	func() {
		defer func() {
			panicErrGot = recover()
		}()

		req.Finish(func() {
			// verify that total requests executing goes up by 1 since the request is executing.
			if queue.totRequestsExecuting != 1 {
				t.Fatalf("expected total requests currently executing of the QueueSet to be 1, but got: %d", queue.totRequestsExecuting)
			}

			panic(panicErrExpected)
		})
	}()

	// verify that the panic was from us (above)
	if panicErrExpected != panicErrGot {
		t.Errorf("expected panic error: %#v, but got: %#v", panicErrExpected, panicErrGot)
	}
	if queue.totRequestsExecuting != 0 {
		t.Errorf("expected total requests currently executing of the QueueSet to be 0, but got: %d", queue.totRequestsExecuting)
	}
}

func TestSelectQueueLocked(t *testing.T) {
	var G float64 = 60
	tests := []struct {
		name                    string
		robinIndex              int
		concurrencyLimit        int
		totSeatsInUse           int
		queues                  []*queue
		attempts                int
		beforeSelectQueueLocked func(attempt int, qs *queueSet)
		minQueueIndexExpected   []int
		robinIndexExpected      []int
	}{
		{
			name:             "width=1, seats are available, the queue with less virtual start time wins",
			concurrencyLimit: 1,
			totSeatsInUse:    0,
			robinIndex:       -1,
			queues: []*queue{
				{
					virtualStart: 200,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 1}},
					),
				},
				{
					virtualStart: 100,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 1}},
					),
				},
			},
			attempts:              1,
			minQueueIndexExpected: []int{1},
			robinIndexExpected:    []int{1},
		},
		{
			name:             "width=1, all seats are occupied, no queue is picked",
			concurrencyLimit: 1,
			totSeatsInUse:    1,
			robinIndex:       -1,
			queues: []*queue{
				{
					virtualStart: 200,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 1}},
					),
				},
			},
			attempts:              1,
			minQueueIndexExpected: []int{-1},
			robinIndexExpected:    []int{0},
		},
		{
			name:             "width > 1, seats are available for request with the least finish time, queue is picked",
			concurrencyLimit: 50,
			totSeatsInUse:    25,
			robinIndex:       -1,
			queues: []*queue{
				{
					virtualStart: 200,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 50}},
					),
				},
				{
					virtualStart: 100,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 25}},
					),
				},
			},
			attempts:              1,
			minQueueIndexExpected: []int{1},
			robinIndexExpected:    []int{1},
		},
		{
			name:             "width > 1, seats are not available for request with the least finish time, queue is not picked",
			concurrencyLimit: 50,
			totSeatsInUse:    26,
			robinIndex:       -1,
			queues: []*queue{
				{
					virtualStart: 200,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 10}},
					),
				},
				{
					virtualStart: 100,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 25}},
					),
				},
			},
			attempts:              3,
			minQueueIndexExpected: []int{-1, -1, -1},
			robinIndexExpected:    []int{1, 1, 1},
		},
		{
			name:             "width > 1, seats become available before 3rd attempt, queue is picked",
			concurrencyLimit: 50,
			totSeatsInUse:    26,
			robinIndex:       -1,
			queues: []*queue{
				{
					virtualStart: 200,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 10}},
					),
				},
				{
					virtualStart: 100,
					requests: newFIFO(
						&request{width: fcrequest.Width{Seats: 25}},
					),
				},
			},
			beforeSelectQueueLocked: func(attempt int, qs *queueSet) {
				if attempt == 3 {
					qs.totSeatsInUse = 25
				}
			},
			attempts:              3,
			minQueueIndexExpected: []int{-1, -1, 1},
			robinIndexExpected:    []int{1, 1, 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			qs := &queueSet{
				estimatedServiceTime: G,
				robinIndex:           test.robinIndex,
				totSeatsInUse:        test.totSeatsInUse,
				dCfg: fq.DispatchingConfig{
					ConcurrencyLimit: test.concurrencyLimit,
				},
				queues: test.queues,
			}

			t.Logf("QS: robin index=%d, seats in use=%d limit=%d", qs.robinIndex, qs.totSeatsInUse, qs.dCfg.ConcurrencyLimit)

			for i := 0; i < test.attempts; i++ {
				attempt := i + 1
				if test.beforeSelectQueueLocked != nil {
					test.beforeSelectQueueLocked(attempt, qs)
				}

				var minQueueExpected *queue
				if queueIdx := test.minQueueIndexExpected[i]; queueIdx >= 0 {
					minQueueExpected = test.queues[queueIdx]
				}

				minQueueGot := qs.selectQueueLocked()
				if minQueueExpected != minQueueGot {
					t.Errorf("Expected queue: %#v, but got: %#v", minQueueExpected, minQueueGot)
				}

				robinIndexExpected := test.robinIndexExpected[i]
				if robinIndexExpected != qs.robinIndex {
					t.Errorf("Expected robin index: %d for attempt: %d, but got: %d", robinIndexExpected, attempt, qs.robinIndex)
				}
			}
		})
	}
}

func newFIFO(requests ...*request) fifo {
	l := newRequestFIFO()
	for i := range requests {
		l.Enqueue(requests[i])
	}
	return l
}

func newObserverPair(clk clock.PassiveClock) metrics.TimedObserverPair {
	return metrics.PriorityLevelConcurrencyObserverPairGenerator.Generate(1, 1, []string{"test"})
}
