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
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/gin-gonic/gin"
	"github.com/pingcap/errors"
	"go.uber.org/zap"

	"github.com/pingcap/log"
	"github.com/pingcap/tiflow/dm/checker"
	dmconfig "github.com/pingcap/tiflow/dm/config"
	ctlcommon "github.com/pingcap/tiflow/dm/ctl/common"
	"github.com/pingcap/tiflow/dm/master"
	"github.com/pingcap/tiflow/engine/framework"
	"github.com/pingcap/tiflow/engine/framework/logutil"
	frameModel "github.com/pingcap/tiflow/engine/framework/model"
	"github.com/pingcap/tiflow/engine/framework/registry"
	"github.com/pingcap/tiflow/engine/jobmaster/dm/checkpoint"
	"github.com/pingcap/tiflow/engine/jobmaster/dm/config"
	"github.com/pingcap/tiflow/engine/jobmaster/dm/metadata"
	"github.com/pingcap/tiflow/engine/jobmaster/dm/runtime"
	"github.com/pingcap/tiflow/engine/model"
	dcontext "github.com/pingcap/tiflow/engine/pkg/context"
	dmpkg "github.com/pingcap/tiflow/engine/pkg/dm"
	"github.com/pingcap/tiflow/engine/pkg/p2p"
)

// JobMaster defines job master of dm job
type JobMaster struct {
	framework.BaseJobMaster

	// only use when init
	// it will be outdated if user update job cfg.
	initJobCfg *config.JobCfg
	// taskID -> FinishedTaskStatus
	// worker exists after finished, so we need record the finished status for QueryJobStatus
	// finishedStatus will be reset when jobmaster failover and update-job-cfg request comes.
	finishedStatus sync.Map

	metadata              *metadata.MetaData
	workerManager         *WorkerManager
	taskManager           *TaskManager
	messageAgent          dmpkg.MessageAgent
	checkpointAgent       checkpoint.Agent
	messageHandlerManager p2p.MessageHandlerManager
}

var (
	_               framework.JobMasterImpl = (*JobMaster)(nil)
	internalVersion                         = semver.New("6.1.0")
)

type dmJobMasterFactory struct{}

// RegisterWorker is used to register dm job master to global registry
func RegisterWorker() {
	registry.GlobalWorkerRegistry().MustRegisterWorkerType(framework.DMJobMaster, dmJobMasterFactory{})
}

// DeserializeConfig implements WorkerFactory.DeserializeConfig
func (j dmJobMasterFactory) DeserializeConfig(configBytes []byte) (registry.WorkerConfig, error) {
	cfg := &config.JobCfg{}
	err := cfg.Decode(configBytes)
	return cfg, err
}

// NewWorkerImpl implements WorkerFactory.NewWorkerImpl
func (j dmJobMasterFactory) NewWorkerImpl(dCtx *dcontext.Context, workerID frameModel.WorkerID, masterID frameModel.MasterID, conf framework.WorkerConfig) (framework.WorkerImpl, error) {
	log.L().Info("new dm jobmaster", zap.String(logutil.ConstFieldJobKey, workerID))
	jm := &JobMaster{
		initJobCfg: conf.(*config.JobCfg),
	}
	// nolint:errcheck
	dCtx.Deps().Construct(func(m p2p.MessageHandlerManager) (p2p.MessageHandlerManager, error) {
		jm.messageHandlerManager = m
		return m, nil
	})
	return jm, nil
}

// initComponents initializes components of dm job master
// it need to be called firstly in InitImpl and OnMasterRecovered
// we should create all components if there is any error
// CloseImpl/StopImpl will be called later to close components
func (jm *JobMaster) initComponents(ctx context.Context) error {
	jm.Logger().Info("initializing the dm jobmaster components")
	taskStatus, workerStatus, err := jm.getInitStatus()
	jm.metadata = metadata.NewMetaData(jm.MetaKVClient(), jm.Logger())
	jm.messageAgent = dmpkg.NewMessageAgent(jm.ID(), jm, jm.messageHandlerManager, jm.Logger())
	jm.checkpointAgent = checkpoint.NewCheckpointAgent(jm.ID(), jm.Logger())
	jm.taskManager = NewTaskManager(taskStatus, jm.metadata.JobStore(), jm.messageAgent, jm.Logger())
	jm.workerManager = NewWorkerManager(jm.ID(), workerStatus, jm.metadata.JobStore(), jm, jm.messageAgent, jm.checkpointAgent, jm.Logger())
	return err
}

// InitImpl implements JobMasterImpl.InitImpl
func (jm *JobMaster) InitImpl(ctx context.Context) error {
	jm.Logger().Info("initializing the dm jobmaster")
	if err := jm.initComponents(ctx); err != nil {
		return err
	}
	if err := jm.preCheck(ctx, jm.initJobCfg); err != nil {
		return jm.Exit(ctx, framework.ExitReasonFailed, err, "")
	}
	if err := jm.bootstrap(ctx); err != nil {
		return err
	}
	if err := jm.checkpointAgent.Create(ctx, jm.initJobCfg); err != nil {
		return err
	}
	return jm.taskManager.OperateTask(ctx, dmpkg.Create, jm.initJobCfg, nil)
}

// Tick implements JobMasterImpl.Tick
func (jm *JobMaster) Tick(ctx context.Context) error {
	jm.workerManager.Tick(ctx)
	jm.taskManager.Tick(ctx)
	if err := jm.messageAgent.Tick(ctx); err != nil {
		return err
	}
	if jm.isFinished(ctx) {
		return jm.cancel(ctx, frameModel.WorkerStatusFinished)
	}
	return nil
}

// OnMasterRecovered implements JobMasterImpl.OnMasterRecovered
// When it is called, the jobCfg may not be in the metadata, and we should not report an error
func (jm *JobMaster) OnMasterRecovered(ctx context.Context) error {
	jm.Logger().Info("recovering the dm jobmaster")
	if err := jm.initComponents(ctx); err != nil {
		return err
	}
	return jm.bootstrap(ctx)
}

// OnWorkerDispatched implements JobMasterImpl.OnWorkerDispatched
func (jm *JobMaster) OnWorkerDispatched(worker framework.WorkerHandle, result error) error {
	jm.Logger().Info("on worker dispatched", zap.String(logutil.ConstFieldWorkerKey, worker.ID()))
	if result != nil {
		jm.Logger().Error("failed to create worker", zap.String(logutil.ConstFieldWorkerKey, worker.ID()), zap.Error(result))
		jm.workerManager.removeWorkerStatusByWorkerID(worker.ID())
		jm.workerManager.SetNextCheckTime(time.Now())
	}
	return nil
}

// OnWorkerOnline implements JobMasterImpl.OnWorkerOnline
func (jm *JobMaster) OnWorkerOnline(worker framework.WorkerHandle) error {
	jm.Logger().Debug("on worker online", zap.String(logutil.ConstFieldWorkerKey, worker.ID()))
	return jm.handleOnlineStatus(worker)
}

func (jm *JobMaster) handleOnlineStatus(worker framework.WorkerHandle) error {
	var taskStatus runtime.TaskStatus
	if err := json.Unmarshal(worker.Status().ExtBytes, &taskStatus); err != nil {
		return err
	}

	jm.finishedStatus.Delete(taskStatus.Task)
	jm.taskManager.UpdateTaskStatus(taskStatus)
	jm.workerManager.UpdateWorkerStatus(runtime.NewWorkerStatus(taskStatus.Task, taskStatus.Unit, worker.ID(), runtime.WorkerOnline, taskStatus.CfgModRevision))
	return jm.messageAgent.UpdateClient(taskStatus.Task, worker.Unwrap())
}

// OnWorkerOffline implements JobMasterImpl.OnWorkerOffline
func (jm *JobMaster) OnWorkerOffline(worker framework.WorkerHandle, reason error) error {
	jm.Logger().Info("on worker offline", zap.String(logutil.ConstFieldWorkerKey, worker.ID()))
	workerStatus := worker.Status()
	var taskStatus runtime.TaskStatus
	if err := json.Unmarshal(workerStatus.ExtBytes, &taskStatus); err != nil {
		return err
	}

	if taskStatus.Stage == metadata.StageFinished {
		var finishedTaskStatus runtime.FinishedTaskStatus
		if err := json.Unmarshal(workerStatus.ExtBytes, &finishedTaskStatus); err != nil {
			return err
		}
		return jm.onWorkerFinished(finishedTaskStatus, worker)
	}
	jm.taskManager.UpdateTaskStatus(runtime.NewOfflineStatus(taskStatus.Task))
	jm.workerManager.UpdateWorkerStatus(runtime.NewWorkerStatus(taskStatus.Task, taskStatus.Unit, worker.ID(), runtime.WorkerOffline, taskStatus.CfgModRevision))
	if err := jm.messageAgent.UpdateClient(taskStatus.Task, nil); err != nil {
		return err
	}
	jm.workerManager.SetNextCheckTime(time.Now())
	return nil
}

func (jm *JobMaster) onWorkerFinished(finishedTaskStatus runtime.FinishedTaskStatus, worker framework.WorkerHandle) error {
	jm.Logger().Info("on worker finished", zap.String(logutil.ConstFieldWorkerKey, worker.ID()))
	taskStatus := finishedTaskStatus.TaskStatus
	jm.finishedStatus.Store(taskStatus.Task, finishedTaskStatus)
	jm.taskManager.UpdateTaskStatus(taskStatus)
	jm.workerManager.UpdateWorkerStatus(runtime.NewWorkerStatus(taskStatus.Task, taskStatus.Unit, worker.ID(), runtime.WorkerFinished, taskStatus.CfgModRevision))
	if err := jm.messageAgent.RemoveClient(taskStatus.Task); err != nil {
		return err
	}
	jm.workerManager.SetNextCheckTime(time.Now())
	return nil
}

// OnWorkerStatusUpdated implements JobMasterImpl.OnWorkerStatusUpdated
func (jm *JobMaster) OnWorkerStatusUpdated(worker framework.WorkerHandle, newStatus *frameModel.WorkerStatus) error {
	// we alreay update finished status in OnWorkerOffline
	if newStatus.Code == frameModel.WorkerStatusFinished || len(newStatus.ExtBytes) == 0 {
		return nil
	}
	jm.Logger().Info("on worker status updated", zap.String(logutil.ConstFieldWorkerKey, worker.ID()), zap.String("extra bytes", string(newStatus.ExtBytes)))
	if err := jm.handleOnlineStatus(worker); err != nil {
		return err
	}
	// run task manager tick when worker status changed to operate task.
	jm.taskManager.SetNextCheckTime(time.Now())
	return nil
}

// OnJobManagerMessage implements JobMasterImpl.OnJobManagerMessage
func (jm *JobMaster) OnJobManagerMessage(topic p2p.Topic, message interface{}) error {
	// TODO: receive user request
	return nil
}

// OnOpenAPIInitialized implements JobMasterImpl.OnOpenAPIInitialized.
func (jm *JobMaster) OnOpenAPIInitialized(router *gin.RouterGroup) {
	jm.initOpenAPI(router)
}

// OnWorkerMessage implements JobMasterImpl.OnWorkerMessage
func (jm *JobMaster) OnWorkerMessage(worker framework.WorkerHandle, topic p2p.Topic, message interface{}) error {
	return nil
}

// OnMasterMessage implements JobMasterImpl.OnMasterMessage
func (jm *JobMaster) OnMasterMessage(topic p2p.Topic, message interface{}) error {
	return nil
}

// CloseImpl implements JobMasterImpl.CloseImpl
func (jm *JobMaster) CloseImpl(ctx context.Context) error {
	return jm.messageAgent.Close(ctx)
}

// OnCancel implements JobMasterImpl.OnCancel
func (jm *JobMaster) OnCancel(ctx context.Context) error {
	jm.Logger().Info("on cancel job master")
	return jm.cancel(ctx, frameModel.WorkerStatusStopped)
}

// StopImpl implements JobMasterImpl.StopImpl
func (jm *JobMaster) StopImpl(ctx context.Context) error {
	jm.Logger().Info("stoping the dm jobmaster")

	// close component
	if err := jm.CloseImpl(ctx); err != nil {
		jm.Logger().Error("failed to close dm jobmaster", zap.Error(err))
		return err
	}

	// remove other resources
	if err := jm.removeCheckpoint(ctx); err != nil {
		// log and ignore the error.
		jm.Logger().Error("failed to remove checkpoint", zap.Error(err))
	}
	return jm.taskManager.OperateTask(ctx, dmpkg.Delete, nil, nil)
}

// Workload implements JobMasterImpl.Workload
func (jm *JobMaster) Workload() model.RescUnit {
	// TODO: implement workload
	return 2
}

// IsJobMasterImpl implements JobMasterImpl.IsJobMasterImpl
func (jm *JobMaster) IsJobMasterImpl() {
	panic("unreachable")
}

func (jm *JobMaster) getInitStatus() ([]runtime.TaskStatus, []runtime.WorkerStatus, error) {
	jm.Logger().Info("get init status")
	// NOTE: GetWorkers should return all online workers,
	// and no further OnWorkerOnline will be received if JobMaster doesn't CreateWorker.
	workerHandles := jm.GetWorkers()
	taskStatusList := make([]runtime.TaskStatus, 0, len(workerHandles))
	workerStatusList := make([]runtime.WorkerStatus, 0, len(workerHandles))
	for _, workerHandle := range workerHandles {
		if workerHandle.GetTombstone() != nil {
			continue
		}
		var taskStatus runtime.TaskStatus
		err := json.Unmarshal(workerHandle.Status().ExtBytes, &taskStatus)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		taskStatusList = append(taskStatusList, taskStatus)
		workerStatusList = append(workerStatusList, runtime.NewWorkerStatus(taskStatus.Task, taskStatus.Unit, workerHandle.ID(), runtime.WorkerOnline, taskStatus.CfgModRevision))
	}

	return taskStatusList, workerStatusList, nil
}

func (jm *JobMaster) preCheck(ctx context.Context, cfg *config.JobCfg) error {
	jm.Logger().Info("start pre-checking job config")

	// TODO: refactor this, e.g. move this check to checkpoint agent
	// lightning create checkpoint table with name `$jobID_lightning_checkpoint_list`
	// max table of TiDB is 64, so length of jobID should be less or equal than 64-26=38
	if len(jm.ID()) > 38 {
		return errors.New("job id is too long, max length is 38")
	}

	if err := master.AdjustTargetDB(ctx, cfg.TargetDB); err != nil {
		return err
	}

	taskCfgs := cfg.ToTaskCfgs()
	dmSubtaskCfgs := make([]*dmconfig.SubTaskConfig, 0, len(taskCfgs))
	for _, taskCfg := range taskCfgs {
		dmSubtaskCfgs = append(dmSubtaskCfgs, taskCfg.ToDMSubTaskCfg(jm.ID()))
	}

	msg, err := checker.CheckSyncConfigFunc(ctx, dmSubtaskCfgs, ctlcommon.DefaultErrorCnt, ctlcommon.DefaultWarnCnt)
	if err != nil {
		jm.Logger().Error("error when pre-checking", zap.Error(err))
		return err
	}
	jm.Logger().Info("finish pre-checking job config", zap.String("result", msg))
	return nil
}

// all task finished and all worker tombstone
func (jm *JobMaster) isFinished(ctx context.Context) bool {
	return jm.taskManager.allFinished(ctx) && jm.workerManager.allTombStone()
}

func (jm *JobMaster) status(ctx context.Context, code frameModel.WorkerStatusCode) (frameModel.WorkerStatus, error) {
	status := frameModel.WorkerStatus{
		Code: code,
	}
	if jobStatus, err := jm.QueryJobStatus(ctx, nil); err != nil {
		return status, err
	} else if bs, err := json.Marshal(jobStatus); err != nil {
		return status, err
	} else {
		status.ExtBytes = bs
		return status, nil
	}
}

// cancel remove jobCfg in metadata, and wait all workers offline.
func (jm *JobMaster) cancel(ctx context.Context, code frameModel.WorkerStatusCode) error {
	var extMsg string
	status, err := jm.status(ctx, code)
	if err != nil {
		jm.Logger().Error("failed to get status", zap.Error(err))
	} else {
		extMsg = string(status.ExtBytes)
	}

	if err := jm.taskManager.OperateTask(ctx, dmpkg.Deleting, nil, nil); err != nil {
		// would not recover again
		return jm.Exit(ctx, framework.ExitReasonCanceled, err, extMsg)
	}
	// wait all worker exit
	jm.workerManager.SetNextCheckTime(time.Now())
	for {
		select {
		case <-ctx.Done():
			return jm.Exit(ctx, framework.ExitReasonCanceled, ctx.Err(), extMsg)
		case <-time.After(time.Second):
			if jm.workerManager.allTombStone() {
				return jm.Exit(ctx, framework.WorkerStatusCodeToExitReason(status.Code), err, extMsg)
			}
			jm.workerManager.SetNextCheckTime(time.Now())
		}
	}
}

func (jm *JobMaster) removeCheckpoint(ctx context.Context) error {
	state, err := jm.metadata.JobStore().Get(ctx)
	if err != nil {
		return err
	}
	job := state.(*metadata.Job)
	for _, task := range job.Tasks {
		cfg := (*config.JobCfg)(task.Cfg)
		return jm.checkpointAgent.Remove(ctx, cfg)
	}
	return errors.New("no task found in job")
}

// bootstrap should be invoked after initComponents.
func (jm *JobMaster) bootstrap(ctx context.Context) error {
	jm.Logger().Info("start bootstraping")
	// get old version
	clusterInfoStore := jm.metadata.ClusterInfoStore()
	state, err := clusterInfoStore.Get(ctx)
	if err != nil {
		// put cluster info for new job
		// TODO: better error handling by error code.
		if err.Error() == "state not found" {
			jm.Logger().Info("put cluster info for new job", zap.Stringer("internal version", internalVersion))
			return clusterInfoStore.Put(ctx, metadata.NewClusterInfo(*internalVersion))
		}
		jm.Logger().Info("get cluster info error", zap.Error(err))
		return err
	}
	clusterInfo := state.(*metadata.ClusterInfo)
	jm.Logger().Info("get cluster info for job", zap.Any("cluster_info", clusterInfo))

	if err := jm.metadata.Upgrade(ctx, clusterInfo.Version); err != nil {
		return err
	}
	if err := jm.checkpointAgent.Upgrade(ctx, clusterInfo.Version); err != nil {
		return err
	}

	// only update for new version
	if clusterInfo.Version.LessThan(*internalVersion) {
		return clusterInfoStore.UpdateVersion(ctx, *internalVersion)
	}
	return nil
}
