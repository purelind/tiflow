// Copyright 2022 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package servermaster

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phayes/freeport"
	pb "github.com/pingcap/tiflow/engine/enginepb"
	"github.com/pingcap/tiflow/engine/framework"
	frameModel "github.com/pingcap/tiflow/engine/framework/model"
	"github.com/pingcap/tiflow/engine/model"
	"github.com/pingcap/tiflow/engine/pkg/externalresource/manager"
	"github.com/pingcap/tiflow/engine/pkg/notifier"
	"github.com/pingcap/tiflow/engine/pkg/p2p"
	"github.com/pingcap/tiflow/engine/servermaster/cluster"
	"github.com/pingcap/tiflow/engine/servermaster/scheduler"
	"github.com/pingcap/tiflow/pkg/logutil"
	"github.com/stretchr/testify/require"
)

func init() {
	err := logutil.InitLogger(&logutil.Config{Level: "warn"})
	if err != nil {
		panic(err)
	}
}

func prepareServerEnv(t *testing.T) *Config {
	ports, err := freeport.GetFreePorts(1)
	require.NoError(t, err)
	cfgTpl := `
addr = "127.0.0.1:%d"
advertise-addr = "127.0.0.1:%d"
[framework-metastore-conf]
store-id = "root"
endpoints = ["127.0.0.1:%d"]
schema = "test0"
auth.user = "root"
[business-metastore-conf]
store-id = "default"
endpoints = ["127.0.0.1:%d"]
schema = "test1"
`
	cfgStr := fmt.Sprintf(cfgTpl, ports[0], ports[0], ports[0], ports[0])
	cfg := GetDefaultMasterConfig()
	err = cfg.configFromString(cfgStr)
	require.Nil(t, err)
	err = cfg.AdjustAndValidate()
	require.Nil(t, err)

	cfg.Addr = fmt.Sprintf("127.0.0.1:%d", ports[0])

	return cfg
}

// Disable parallel run for this case, because prometheus http handler will meet
// data race if parallel run is enabled
func TestServe(t *testing.T) {
	cfg := prepareServerEnv(t)
	s := &Server{
		cfg:        cfg,
		msgService: p2p.NewMessageRPCServiceWithRPCServer("servermaster", nil, nil),
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.serve(ctx)
	}()

	require.Eventually(t, func() bool {
		conn, err := net.Dial("tcp", cfg.Addr)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, time.Second*5, time.Millisecond*100, "wait for server to start")

	apiURL := "http://" + cfg.Addr
	testPprof(t, apiURL)
	testPrometheusMetrics(t, apiURL)

	cancel()
	wg.Wait()
}

func testPprof(t *testing.T, addr string) {
	urls := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
		// enable these two apis will make ut slow
		//"/debug/pprof/profile", http.MethodGet,
		//"/debug/pprof/trace", http.MethodGet,
		"/debug/pprof/threadcreate",
		"/debug/pprof/allocs",
		"/debug/pprof/block",
		"/debug/pprof/goroutine?debug=1",
		"/debug/pprof/mutex?debug=1",
	}
	for _, uri := range urls {
		resp, err := http.Get(addr + uri)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
}

func testPrometheusMetrics(t *testing.T, addr string) {
	resp, err := http.Get(addr + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
}

// Server master requires etcd/gRPC service as the minimum running environment,
// this case
//   - starts an embed etcd with gRPC service, including message service and
//     server master pb service.
//   - campaigns to be leader and then runs leader service.
//
// Disable parallel run for this case, because prometheus http handler will meet
// data race if parallel run is enabled
// FIXME: disable this test temporary for no proper mock of frame metastore
// nolint: deadcode
func testRunLeaderService(t *testing.T) {
	cfg := prepareServerEnv(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := NewServer(cfg, nil)
	require.NoError(t, err)

	_ = s.registerMetaStore(ctx)

	s.initResourceManagerService()
	s.scheduler = scheduler.NewScheduler(
		s.executorManager,
		s.resourceManagerService)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.serve(ctx)
	}()

	sessionCfg, err := s.generateSessionConfig()
	require.NoError(t, err)
	session, err := cluster.NewEtcdSession(ctx, s.etcdClient, sessionCfg)
	require.NoError(t, err)

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.msgService.GetMessageServer().Run(ctx)
	}()

	_, _, err = session.Campaign(ctx, time.Second)
	require.NoError(t, err)

	ctx1, cancel1 := context.WithTimeout(ctx, time.Second)
	defer cancel1()
	err = s.runLeaderService(ctx1)
	require.EqualError(t, err, context.DeadlineExceeded.Error())

	// runLeaderService exits, try to campaign to be leader and run leader servcie again
	_, _, err = session.Campaign(ctx, time.Second)
	require.NoError(t, err)
	ctx2, cancel2 := context.WithTimeout(ctx, time.Second)
	defer cancel2()
	err = s.runLeaderService(ctx2)
	require.EqualError(t, err, context.DeadlineExceeded.Error())

	cancel()
	wg.Wait()
}

type mockJobManager struct {
	framework.BaseMaster
	pb.UnimplementedJobManagerServer
	jobMu sync.RWMutex
	jobs  map[pb.Job_Status]int
}

func (m *mockJobManager) JobCount(status pb.Job_Status) int {
	m.jobMu.RLock()
	defer m.jobMu.RUnlock()
	return m.jobs[status]
}

func (m *mockJobManager) GetJobMasterForwardAddress(ctx context.Context, jobID string) (string, error) {
	panic("not implemented")
}

func (m *mockJobManager) GetJobStatuses(ctx context.Context) (map[frameModel.MasterID]frameModel.MasterStatusCode, error) {
	panic("not implemented")
}

func (m *mockJobManager) WatchJobStatuses(
	ctx context.Context,
) (manager.JobStatusesSnapshot, *notifier.Receiver[manager.JobStatusChangeEvent], error) {
	// TODO implement me
	panic("implement me")
}

type mockExecutorManager struct {
	ExecutorManager
	executorMu sync.RWMutex
	count      map[model.ExecutorStatus]int
}

func (m *mockExecutorManager) Stop() {
}

func (m *mockExecutorManager) ExecutorCount(status model.ExecutorStatus) int {
	m.executorMu.RLock()
	defer m.executorMu.RUnlock()
	return m.count[status]
}

func TestCollectMetric(t *testing.T) {
	cfg := prepareServerEnv(t)

	s := &Server{
		cfg:        cfg,
		metrics:    newServerMasterMetric(),
		msgService: p2p.NewMessageRPCServiceWithRPCServer("servermaster", nil, nil),
	}
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.serve(ctx)
	}()

	jobManager := &mockJobManager{
		jobs: map[pb.Job_Status]int{
			pb.Job_Running: 3,
		},
	}
	executorManager := &mockExecutorManager{
		count: map[model.ExecutorStatus]int{
			model.Initing: 1,
			model.Running: 2,
		},
	}
	s.jobManager = jobManager
	s.executorManager = executorManager

	s.collectLeaderMetric()
	apiURL := fmt.Sprintf("http://%s", cfg.Addr)
	testCustomedPrometheusMetrics(t, apiURL)

	cancel()
	wg.Wait()
	s.Stop()
}

func testCustomedPrometheusMetrics(t *testing.T, addr string) {
	require.Eventually(t, func() bool {
		resp, err := http.Get(addr + "/metrics")
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		metric := string(body)
		return strings.Contains(metric, "dataflow_server_master_job_num") &&
			strings.Contains(metric, "dataflow_server_master_executor_num")
	}, time.Second, time.Millisecond*20)
}
