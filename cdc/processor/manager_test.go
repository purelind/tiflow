// Copyright 2021 PingCAP, Inc.
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

package processor

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/pingcap/errors"
	"github.com/pingcap/tiflow/cdc/model"
	tablepipeline "github.com/pingcap/tiflow/cdc/processor/pipeline"
	"github.com/pingcap/tiflow/pkg/config"
	cdcContext "github.com/pingcap/tiflow/pkg/context"
	cerrors "github.com/pingcap/tiflow/pkg/errors"
	"github.com/pingcap/tiflow/pkg/etcd"
	"github.com/pingcap/tiflow/pkg/orchestrator"
	"github.com/pingcap/tiflow/pkg/upstream"
	"github.com/stretchr/testify/require"
)

type managerTester struct {
	manager  *managerImpl
	state    *orchestrator.GlobalReactorState
	tester   *orchestrator.ReactorStateTester
	liveness model.Liveness
}

// NewManager4Test creates a new processor manager for test
func NewManager4Test(
	t *testing.T,
	createTablePipeline func(ctx cdcContext.Context, tableID model.TableID, replicaInfo *model.TableReplicaInfo) (tablepipeline.TablePipeline, error),
	liveness *model.Liveness,
) *managerImpl {
	captureInfo := &model.CaptureInfo{ID: "capture-test", AdvertiseAddr: "127.0.0.1:0000"}
	m := NewManager(captureInfo, upstream.NewManager4Test(nil), liveness).(*managerImpl)
	m.newProcessor = func(
		state *orchestrator.ChangefeedReactorState,
		captureInfo *model.CaptureInfo,
		changefeedID model.ChangeFeedID,
		up *upstream.Upstream,
		liveness *model.Liveness,
	) *processor {
		return newProcessor4Test(t, state, captureInfo, createTablePipeline, m.liveness)
	}
	return m
}

func (s *managerTester) resetSuit(ctx cdcContext.Context, t *testing.T) {
	s.manager = NewManager4Test(t, func(ctx cdcContext.Context, tableID model.TableID, replicaInfo *model.TableReplicaInfo) (tablepipeline.TablePipeline, error) {
		return &mockTablePipeline{
			tableID:      tableID,
			name:         fmt.Sprintf("`test`.`table%d`", tableID),
			state:        tablepipeline.TableStateReplicating,
			resolvedTs:   replicaInfo.StartTs,
			checkpointTs: replicaInfo.StartTs,
		}, nil
	}, &s.liveness)
	s.state = orchestrator.NewGlobalState(etcd.DefaultCDCClusterID)
	captureInfoBytes, err := ctx.GlobalVars().CaptureInfo.Marshal()
	require.Nil(t, err)
	s.tester = orchestrator.NewReactorStateTester(t, s.state, map[string]string{
		fmt.Sprintf("%s/capture/%s",
			etcd.DefaultClusterAndMetaPrefix,
			ctx.GlobalVars().CaptureInfo.ID): string(captureInfoBytes),
	})
}

func TestChangefeed(t *testing.T) {
	ctx := cdcContext.NewBackendContext4Test(false)
	s := &managerTester{}
	s.resetSuit(ctx, t)
	var err error

	// no changefeed
	_, err = s.manager.Tick(ctx, s.state)
	require.Nil(t, err)

	changefeedID := model.DefaultChangeFeedID("test-changefeed")
	// an inactive changefeed
	s.state.Changefeeds[changefeedID] = orchestrator.NewChangefeedReactorState(
		etcd.DefaultCDCClusterID, changefeedID)
	_, err = s.manager.Tick(ctx, s.state)
	s.tester.MustApplyPatches()
	require.Nil(t, err)
	require.Len(t, s.manager.processors, 0)

	// an active changefeed
	s.state.Changefeeds[changefeedID].PatchInfo(
		func(info *model.ChangeFeedInfo) (*model.ChangeFeedInfo, bool, error) {
			return &model.ChangeFeedInfo{
				SinkURI:    "blackhole://",
				CreateTime: time.Now(),
				StartTs:    0,
				TargetTs:   math.MaxUint64,
				Config:     config.GetDefaultReplicaConfig(),
			}, true, nil
		})
	s.state.Changefeeds[changefeedID].PatchStatus(
		func(status *model.ChangeFeedStatus) (*model.ChangeFeedStatus, bool, error) {
			return &model.ChangeFeedStatus{}, true, nil
		})
	s.tester.MustApplyPatches()
	_, err = s.manager.Tick(ctx, s.state)
	s.tester.MustApplyPatches()
	require.Nil(t, err)
	require.Len(t, s.manager.processors, 1)

	// processor return errors
	s.state.Changefeeds[changefeedID].PatchStatus(
		func(status *model.ChangeFeedStatus) (*model.ChangeFeedStatus, bool, error) {
			status.AdminJobType = model.AdminStop
			return status, true, nil
		})
	s.tester.MustApplyPatches()
	_, err = s.manager.Tick(ctx, s.state)
	s.tester.MustApplyPatches()
	require.Nil(t, err)
	require.Len(t, s.manager.processors, 0)
}

func TestDebugInfo(t *testing.T) {
	ctx := cdcContext.NewBackendContext4Test(false)
	s := &managerTester{}
	s.resetSuit(ctx, t)
	var err error

	// no changefeed
	_, err = s.manager.Tick(ctx, s.state)
	require.Nil(t, err)

	changefeedID := model.DefaultChangeFeedID("test-changefeed")
	// an active changefeed
	s.state.Changefeeds[changefeedID] = orchestrator.NewChangefeedReactorState(
		etcd.DefaultCDCClusterID, changefeedID)
	s.state.Changefeeds[changefeedID].PatchInfo(
		func(info *model.ChangeFeedInfo) (*model.ChangeFeedInfo, bool, error) {
			return &model.ChangeFeedInfo{
				SinkURI:    "blackhole://",
				CreateTime: time.Now(),
				StartTs:    1,
				TargetTs:   math.MaxUint64,
				Config:     config.GetDefaultReplicaConfig(),
			}, true, nil
		})
	s.state.Changefeeds[changefeedID].PatchStatus(
		func(status *model.ChangeFeedStatus) (*model.ChangeFeedStatus, bool, error) {
			return &model.ChangeFeedStatus{}, true, nil
		})
	s.tester.MustApplyPatches()
	_, err = s.manager.Tick(ctx, s.state)
	require.Nil(t, err)
	s.tester.MustApplyPatches()
	require.Len(t, s.manager.processors, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, err = s.manager.Tick(ctx, s.state)
			if err != nil {
				require.True(t, cerrors.ErrReactorFinished.Equal(errors.Cause(err)))
				return
			}
			require.Nil(t, err)
			s.tester.MustApplyPatches()
		}
	}()
	doneM := make(chan error, 1)
	buf := bytes.NewBufferString("")
	s.manager.WriteDebugInfo(ctx, buf, doneM)
	<-doneM
	require.Greater(t, len(buf.String()), 0)
	s.manager.AsyncClose()
	<-done
}

func TestClose(t *testing.T) {
	ctx := cdcContext.NewBackendContext4Test(false)
	s := &managerTester{}
	s.resetSuit(ctx, t)
	var err error

	// no changefeed
	_, err = s.manager.Tick(ctx, s.state)
	require.Nil(t, err)

	changefeedID := model.DefaultChangeFeedID("test-changefeed")
	// an active changefeed
	s.state.Changefeeds[changefeedID] = orchestrator.NewChangefeedReactorState(
		etcd.DefaultCDCClusterID, changefeedID)
	s.state.Changefeeds[changefeedID].PatchInfo(
		func(info *model.ChangeFeedInfo) (*model.ChangeFeedInfo, bool, error) {
			return &model.ChangeFeedInfo{
				SinkURI:    "blackhole://",
				CreateTime: time.Now(),
				StartTs:    0,
				TargetTs:   math.MaxUint64,
				Config:     config.GetDefaultReplicaConfig(),
			}, true, nil
		})
	s.state.Changefeeds[changefeedID].PatchStatus(
		func(status *model.ChangeFeedStatus) (*model.ChangeFeedStatus, bool, error) {
			return &model.ChangeFeedStatus{}, true, nil
		})
	s.tester.MustApplyPatches()
	_, err = s.manager.Tick(ctx, s.state)
	require.Nil(t, err)
	s.tester.MustApplyPatches()
	require.Len(t, s.manager.processors, 1)

	s.manager.AsyncClose()
	_, err = s.manager.Tick(ctx, s.state)
	require.True(t, cerrors.ErrReactorFinished.Equal(errors.Cause(err)))
	s.tester.MustApplyPatches()
	require.Len(t, s.manager.processors, 0)
}

func TestSendCommandError(t *testing.T) {
	liveness := model.LivenessCaptureAlive
	m := NewManager(&model.CaptureInfo{ID: "capture-test"}, nil, &liveness).(*managerImpl)
	ctx, cancel := context.WithCancel(context.TODO())
	cancel()
	// Use unbuffered channel to stable test.
	m.commandQueue = make(chan *command)
	done := make(chan error, 1)
	err := m.sendCommand(ctx, commandTpClose, nil, done)
	require.Error(t, err)
	select {
	case <-done:
	case <-time.After(time.Second):
		require.FailNow(t, "done must be closed")
	}
}

func TestManagerLiveness(t *testing.T) {
	ctx := cdcContext.NewBackendContext4Test(false)
	s := &managerTester{}
	s.resetSuit(ctx, t)
	var err error

	changefeedID := model.DefaultChangeFeedID("test-changefeed")

	// no changefeed
	_, err = s.manager.Tick(ctx, s.state)
	require.Nil(t, err)
	// an inactive changefeed
	s.state.Changefeeds[changefeedID] = orchestrator.NewChangefeedReactorState(
		etcd.DefaultCDCClusterID, changefeedID)
	_, err = s.manager.Tick(ctx, s.state)
	s.tester.MustApplyPatches()
	require.Nil(t, err)
	require.Len(t, s.manager.processors, 0)
	// an active changefeed
	s.state.Changefeeds[changefeedID].PatchInfo(
		func(info *model.ChangeFeedInfo) (*model.ChangeFeedInfo, bool, error) {
			return &model.ChangeFeedInfo{
				SinkURI:    "blackhole://",
				CreateTime: time.Now(),
				StartTs:    0,
				TargetTs:   math.MaxUint64,
				Config:     config.GetDefaultReplicaConfig(),
			}, true, nil
		})
	s.state.Changefeeds[changefeedID].PatchStatus(
		func(status *model.ChangeFeedStatus) (*model.ChangeFeedStatus, bool, error) {
			return &model.ChangeFeedStatus{}, true, nil
		})
	s.tester.MustApplyPatches()
	_, err = s.manager.Tick(ctx, s.state)
	s.tester.MustApplyPatches()
	require.Nil(t, err)
	require.Len(t, s.manager.processors, 1)

	p := s.manager.processors[changefeedID]
	require.Equal(t, model.LivenessCaptureAlive, p.liveness.Load())
	s.liveness.Store(model.LivenessCaptureStopping)
	require.Equal(t, model.LivenessCaptureStopping, p.liveness.Load())
}

func TestQueryTableCount(t *testing.T) {
	liveness := model.LivenessCaptureAlive
	m := NewManager(&model.CaptureInfo{ID: "capture-test"}, nil, &liveness).(*managerImpl)
	ctx := context.TODO()
	// Add some tables to processor.
	m.processors[model.ChangeFeedID{ID: "test"}] = &processor{
		tables: map[model.TableID]tablepipeline.TablePipeline{1: nil, 2: nil},
	}

	done := make(chan error, 1)
	tableCh := make(chan int, 1)
	err := m.sendCommand(ctx, commandTpQueryTableCount, tableCh, done)
	require.Nil(t, err)
	err = m.handleCommand()
	require.Nil(t, err)
	select {
	case count := <-tableCh:
		require.Equal(t, 2, count)
	case <-time.After(time.Second):
		require.FailNow(t, "done must be closed")
	}
}
