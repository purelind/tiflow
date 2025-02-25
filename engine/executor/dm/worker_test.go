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

package dm

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"go.uber.org/dig"

	dmconfig "github.com/pingcap/tiflow/dm/config"
	dmmaster "github.com/pingcap/tiflow/dm/master"
	"github.com/pingcap/tiflow/dm/pb"
	"github.com/pingcap/tiflow/engine/framework"
	"github.com/pingcap/tiflow/engine/framework/registry"
	"github.com/pingcap/tiflow/engine/jobmaster/dm/config"
	"github.com/pingcap/tiflow/engine/jobmaster/dm/metadata"
	"github.com/pingcap/tiflow/engine/model"
	dcontext "github.com/pingcap/tiflow/engine/pkg/context"
	"github.com/pingcap/tiflow/engine/pkg/deps"
	"github.com/pingcap/tiflow/engine/pkg/externalresource/broker"
	kvmock "github.com/pingcap/tiflow/engine/pkg/meta/mock"
	metaModel "github.com/pingcap/tiflow/engine/pkg/meta/model"
	pkgOrm "github.com/pingcap/tiflow/engine/pkg/orm"
	"github.com/pingcap/tiflow/engine/pkg/p2p"
	cerrors "github.com/pingcap/tiflow/pkg/errors"
)

var jobTemplatePath = "../../jobmaster/dm/config/job_template.yaml"

type workerParamListForTest struct {
	dig.Out

	MessageHandlerManager p2p.MessageHandlerManager
	MessageSender         p2p.MessageSender
	FrameMetaClient       pkgOrm.Client
	BusinessClientConn    metaModel.ClientConn
	ResourceBroker        broker.Broker
}

// Init -> Poll -> Close
func TestFactory(t *testing.T) {
	cli, err := pkgOrm.NewMockClient()
	require.NoError(t, err)
	dctx := dcontext.Background()
	dp := deps.NewDeps()
	dctx = dctx.WithDeps(dp)
	messageHandlerManager := p2p.NewMockMessageHandlerManager()
	depsForTest := workerParamListForTest{
		MessageHandlerManager: messageHandlerManager,
		MessageSender:         p2p.NewMockMessageSender(),
		FrameMetaClient:       cli,
		BusinessClientConn:    kvmock.NewMockClientConn(),
		ResourceBroker:        broker.NewBrokerForTesting("exector-id"),
	}
	require.NoError(t, dp.Provide(func() workerParamListForTest {
		return depsForTest
	}))

	funcBackup := dmmaster.CheckAndAdjustSourceConfigFunc
	dmmaster.CheckAndAdjustSourceConfigFunc = func(ctx context.Context, cfg *dmconfig.SourceConfig) error { return nil }
	defer func() {
		dmmaster.CheckAndAdjustSourceConfigFunc = funcBackup
	}()
	// test factory
	var jobCfg config.JobCfg
	require.NoError(t, jobCfg.DecodeFile(jobTemplatePath))
	taskCfg := jobCfg.ToTaskCfgs()["mysql-replica-01"]
	var b bytes.Buffer
	require.NoError(t, toml.NewEncoder(&b).Encode(taskCfg))
	content := b.Bytes()
	RegisterWorker()

	_, err = registry.GlobalWorkerRegistry().CreateWorker(
		dctx, framework.WorkerDMDump, "worker-id", "dm-jobmaster-id",
		content, int64(2))
	require.NoError(t, err)
	_, err = registry.GlobalWorkerRegistry().CreateWorker(
		dctx, framework.WorkerDMLoad, "worker-id", "dm-jobmaster-id",
		content, int64(3))
	require.NoError(t, err)
	_, err = registry.GlobalWorkerRegistry().CreateWorker(
		dctx, framework.WorkerDMSync, "worker-id", "dm-jobmaster-id",
		content, int64(4))
	require.NoError(t, err)
}

func TestWorker(t *testing.T) {
	dctx := dcontext.Background()
	dp := deps.NewDeps()
	dctx = dctx.WithDeps(dp)
	require.NoError(t, dp.Provide(func() p2p.MessageHandlerManager {
		return p2p.NewMockMessageHandlerManager()
	}))
	dmWorker := newDMWorker(dctx, "master-id", framework.WorkerDMDump, &dmconfig.SubTaskConfig{}, 0)
	unitHolder := &mockUnitHolder{}
	dmWorker.unitHolder = unitHolder
	dmWorker.BaseWorker = framework.MockBaseWorker("worker-id", "master-id", dmWorker)
	require.NoError(t, dmWorker.Init(context.Background()))
	// tick
	unitHolder.On("Stage").Return(metadata.StageRunning, nil).Twice()
	unitHolder.On("CheckAndUpdateStatus")
	require.NoError(t, dmWorker.Tick(context.Background()))
	unitHolder.On("Stage").Return(metadata.StageRunning, nil).Twice()
	require.NoError(t, dmWorker.Tick(context.Background()))

	// auto resume error
	unitHolder.On("Stage").Return(metadata.StageError, &pb.ProcessResult{Errors: []*pb.ProcessError{{ErrCode: 0}}}).Twice()
	require.NoError(t, dmWorker.Tick(context.Background()))
	time.Sleep(time.Second)
	unitHolder.On("Stage").Return(metadata.StageError, &pb.ProcessResult{Errors: []*pb.ProcessError{{ErrCode: 0}}}).Once()
	unitHolder.On("Resume").Return(errors.New("resume error")).Once()
	require.EqualError(t, dmWorker.Tick(context.Background()), "resume error")
	// auto resume normal
	unitHolder.On("Stage").Return(metadata.StageError, &pb.ProcessResult{Errors: []*pb.ProcessError{{ErrCode: 0}}}).Once()
	unitHolder.On("Stage").Return(metadata.StageRunning, nil).Once()
	unitHolder.On("Resume").Return(nil).Once()
	require.NoError(t, dmWorker.Tick(context.Background()))

	// placeholder
	require.Equal(t, model.RescUnit(0), dmWorker.Workload())
	require.NoError(t, dmWorker.OnMasterFailover(framework.MasterFailoverReason{}))
	require.NoError(t, dmWorker.OnMasterMessage("", nil))

	// Finished
	unitHolder.On("Stage").Return(metadata.StageFinished, nil).Times(3)
	unitHolder.On("Status").Return(&pb.DumpStatus{}).Once()
	require.True(t, cerrors.ErrWorkerFinish.Equal(dmWorker.Tick(context.Background())))

	unitHolder.AssertExpectations(t)
}
