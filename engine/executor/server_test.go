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

package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/phayes/freeport"
	"github.com/pingcap/errors"
	"github.com/pingcap/tiflow/engine/model"
	"github.com/pingcap/tiflow/engine/pkg/client"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/pingcap/tiflow/engine/enginepb"
	"github.com/pingcap/tiflow/engine/executor/server"
	"github.com/pingcap/tiflow/engine/executor/worker"
	"github.com/pingcap/tiflow/pkg/logutil"
	"github.com/pingcap/tiflow/pkg/uuid"
)

func init() {
	err := logutil.InitLogger(&logutil.Config{Level: "warn"})
	if err != nil {
		panic(err)
	}
}

func TestStartTCPSrv(t *testing.T) {
	t.Parallel()

	cfg := GetDefaultExecutorConfig()
	port, err := freeport.GetFreePort()
	require.Nil(t, err)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cfg.Addr = addr
	s := NewServer(cfg, nil)

	s.grpcSrv = grpc.NewServer()
	wg, ctx := errgroup.WithContext(context.Background())
	err = s.startTCPService(ctx, wg)
	require.Nil(t, err)

	apiURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	testPprof(t, apiURL)

	testPrometheusMetrics(t, apiURL)
	s.Stop()
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
		require.Nil(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_, err = io.ReadAll(resp.Body)
		require.Nil(t, err)
	}
}

func testPrometheusMetrics(t *testing.T, addr string) {
	urls := []string{
		"/metrics",
	}
	for _, uri := range urls {
		resp, err := http.Get(addr + uri)
		require.Nil(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_, err = io.ReadAll(resp.Body)
		require.Nil(t, err)
	}
}

func TestCollectMetric(t *testing.T) {
	wg, ctx := errgroup.WithContext(context.Background())
	cfg := GetDefaultExecutorConfig()
	port, err := freeport.GetFreePort()
	require.Nil(t, err)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cfg.Addr = addr
	s := NewServer(cfg, nil)
	s.taskRunner = worker.NewTaskRunner(defaultRuntimeIncomingQueueLen, defaultRuntimeInitConcurrency)

	s.grpcSrv = grpc.NewServer()
	err = s.startTCPService(ctx, wg)
	require.Nil(t, err)

	wg.Go(func() error {
		return s.collectMetricLoop(ctx, time.Millisecond*10)
	})
	apiURL := fmt.Sprintf("http://%s", addr)
	testCustomedPrometheusMetrics(t, apiURL)
	s.Stop()
	wg.Wait()
}

func testCustomedPrometheusMetrics(t *testing.T, addr string) {
	require.Eventually(t, func() bool {
		resp, err := http.Get(addr + "/metrics")
		require.Nil(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.Nil(t, err)
		metric := string(body)
		return strings.Contains(metric, "dataflow_executor_task_num")
	}, time.Second, time.Millisecond*20)
}

type registerExecutorReturnValue struct {
	executor *pb.Executor
	err      error
}

type mockRegisterMasterClient struct {
	client.ServerMasterClient
	respChan chan *registerExecutorReturnValue
}

func newMockRegisterMasterClient(chanBufferSize int) *mockRegisterMasterClient {
	return &mockRegisterMasterClient{
		respChan: make(chan *registerExecutorReturnValue, chanBufferSize),
	}
}

func (c *mockRegisterMasterClient) RegisterExecutor(
	ctx context.Context, req *pb.RegisterExecutorRequest,
) (nodeID model.ExecutorID, err error) {
	value := <-c.respChan
	if value.err != nil {
		return "", value.err
	}
	return model.ExecutorID(value.executor.Id), nil
}

func TestSelfRegister(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := GetDefaultExecutorConfig()
	port, err := freeport.GetFreePort()
	require.Nil(t, err)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cfg.AdvertiseAddr = addr
	s := NewServer(cfg, nil)
	mockMasterClient := newMockRegisterMasterClient(10)
	s.masterClient = mockMasterClient

	mockMasterClient.respChan <- &registerExecutorReturnValue{
		nil, errors.New("service unavailable"),
	}
	err = s.selfRegister(ctx)
	require.Error(t, err, "service unavailable")

	executorID := uuid.NewGenerator().NewString()
	returnValues := []*registerExecutorReturnValue{
		{
			&pb.Executor{
				Id: executorID,
			}, nil,
		},
	}
	for _, val := range returnValues {
		mockMasterClient.respChan <- val
	}
	err = s.selfRegister(ctx)
	require.NoError(t, err)
	require.Equal(t, executorID, string(s.selfID))
}

func TestRPCCallBeforeInitialized(t *testing.T) {
	svr := &Server{
		metastores: server.NewMetastoreManager(),
	}

	_, err := svr.PreDispatchTask(context.Background(), &pb.PreDispatchTaskRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unavailable, status.Convert(err).Code())

	_, err = svr.ConfirmDispatchTask(context.Background(), &pb.ConfirmDispatchTaskRequest{})
	require.Error(t, err)
	require.Equal(t, codes.Unavailable, status.Convert(err).Code())
}
