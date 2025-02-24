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
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	perrors "github.com/pingcap/errors"
	"github.com/pingcap/log"
	"github.com/pingcap/tiflow/dm/common"
	pb "github.com/pingcap/tiflow/engine/enginepb"
	"github.com/pingcap/tiflow/engine/executor/server"
	"github.com/pingcap/tiflow/engine/executor/worker"
	"github.com/pingcap/tiflow/engine/framework"
	frameLog "github.com/pingcap/tiflow/engine/framework/logutil"
	frameModel "github.com/pingcap/tiflow/engine/framework/model"
	"github.com/pingcap/tiflow/engine/framework/registry"
	"github.com/pingcap/tiflow/engine/framework/taskutil"
	"github.com/pingcap/tiflow/engine/internal/pkg/discovery"
	"github.com/pingcap/tiflow/engine/model"
	pkgClient "github.com/pingcap/tiflow/engine/pkg/client"
	dcontext "github.com/pingcap/tiflow/engine/pkg/context"
	"github.com/pingcap/tiflow/engine/pkg/deps"
	"github.com/pingcap/tiflow/engine/pkg/externalresource/broker"
	metaModel "github.com/pingcap/tiflow/engine/pkg/meta/model"
	pkgOrm "github.com/pingcap/tiflow/engine/pkg/orm"
	"github.com/pingcap/tiflow/engine/pkg/p2p"
	"github.com/pingcap/tiflow/engine/pkg/promutil"
	"github.com/pingcap/tiflow/engine/pkg/tenant"
	"github.com/pingcap/tiflow/engine/test"
	"github.com/pingcap/tiflow/engine/test/mock"
	"github.com/pingcap/tiflow/pkg/errors"
	"github.com/pingcap/tiflow/pkg/logutil"
	p2pImpl "github.com/pingcap/tiflow/pkg/p2p"
	"github.com/pingcap/tiflow/pkg/security"
	"github.com/pingcap/tiflow/pkg/tcpserver"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// TODO since we introduced queuing in the TaskRunner, it is no longer
	// easy to implement the capacity. Think of a better solution later.
	// defaultRuntimeCapacity      = 65536
	defaultRuntimeIncomingQueueLen   = 256
	defaultRuntimeInitConcurrency    = 256
	defaultTaskPreDispatchRequestTTL = 10 * time.Second
	defaultDiscoveryAutoSyncInterval = 5 * time.Second
)

// Server is an executor server.
type Server struct {
	cfg     *Config
	testCtx *test.Context

	tcpServer     tcpserver.TCPServer
	grpcSrv       *grpc.Server
	masterClient  pkgClient.ServerMasterClient
	executorGroup *pkgClient.DefaultExecutorGroup
	taskRunner    *worker.TaskRunner
	taskCommitter *worker.TaskCommitter
	msgServer     *p2p.MessageRPCService
	selfID        model.ExecutorID

	lastHearbeatTime time.Time

	mockSrv mock.GrpcServer

	metastores server.MetastoreManager

	p2pMsgRouter   p2pImpl.MessageRouter
	resourceBroker broker.Broker
	jobAPISrv      *jobAPIServer
}

// NewServer creates a new executor server instance
func NewServer(cfg *Config, ctx *test.Context) *Server {
	log.Info("creating executor", zap.Stringer("config", cfg))

	registerWorkerOnce.Do(registerWorkers)
	s := Server{
		cfg:        cfg,
		testCtx:    ctx,
		jobAPISrv:  newJobAPIServer(),
		metastores: server.NewMetastoreManager(),
	}
	return &s
}

func (s *Server) buildDeps() (*deps.Deps, error) {
	deps := deps.NewDeps()
	err := deps.Provide(func() p2p.MessageHandlerManager {
		return s.msgServer.MakeHandlerManager()
	})
	if err != nil {
		return nil, err
	}

	err = deps.Provide(func() p2p.MessageSender {
		return p2p.NewMessageSender(s.p2pMsgRouter)
	})
	if err != nil {
		return nil, err
	}

	cli, err := pkgOrm.NewClient(s.metastores.FrameworkClientConn())
	if err != nil {
		return nil, err
	}
	err = deps.Provide(func() pkgOrm.Client {
		return cli
	})
	if err != nil {
		return nil, err
	}

	err = deps.Provide(func() metaModel.ClientConn {
		return s.metastores.BusinessClientConn()
	})
	if err != nil {
		return nil, err
	}

	err = deps.Provide(func() pkgClient.ExecutorGroup {
		return s.executorGroup
	})
	if err != nil {
		return nil, err
	}

	err = deps.Provide(func() pkgClient.ServerMasterClient {
		return s.masterClient
	})
	if err != nil {
		return nil, err
	}

	err = deps.Provide(func() broker.Broker {
		return s.resourceBroker
	})
	if err != nil {
		return nil, err
	}

	return deps, nil
}

func (s *Server) makeTask(
	ctx context.Context,
	projectInfo *pb.ProjectInfo,
	workerID frameModel.WorkerID,
	masterID frameModel.MasterID,
	workerType frameModel.WorkerType,
	workerConfig []byte,
	workerEpoch frameModel.Epoch,
) (worker.Runnable, error) {
	dctx := dcontext.NewContext(ctx)
	dp, err := s.buildDeps()
	if err != nil {
		return nil, err
	}
	dctx = dctx.WithDeps(dp)
	dctx.Environ.NodeID = p2p.NodeID(s.selfID)
	dctx.Environ.Addr = s.cfg.AdvertiseAddr
	dctx.ProjectInfo = tenant.NewProjectInfo(projectInfo.GetTenantId(), projectInfo.GetProjectId())

	logger := frameLog.WithProjectInfo(logutil.FromContext(ctx), dctx.ProjectInfo)
	logutil.NewContextWithLogger(dctx, logger)

	// NOTICE: only take effect when job type is job master
	masterMeta := &frameModel.MasterMetaKVData{
		ProjectID: dctx.ProjectInfo.UniqueID(),
		ID:        workerID,
		Tp:        workerType,
		Config:    workerConfig,
	}
	metaBytes, err := masterMeta.Marshal()
	if err != nil {
		return nil, err
	}
	dctx.Environ.MasterMetaBytes = metaBytes

	newWorker, err := registry.GlobalWorkerRegistry().CreateWorker(
		dctx,
		workerType,
		workerID,
		masterID,
		workerConfig,
		workerEpoch,
	)
	if err != nil {
		log.Error("Failed to create worker", zap.Error(err))
		return nil, err
	}
	if jm, ok := newWorker.(framework.BaseJobMasterExt); ok {
		jobID := newWorker.ID()
		s.jobAPISrv.initialize(jobID, jm.TriggerOpenAPIInitialize)
	}

	return taskutil.WrapWorker(newWorker), nil
}

// PreDispatchTask implements Executor.PreDispatchTask
func (s *Server) PreDispatchTask(ctx context.Context, req *pb.PreDispatchTaskRequest) (*pb.PreDispatchTaskResponse, error) {
	if !s.isReadyToServe() {
		return nil, status.Error(codes.Unavailable, "executor server is not ready")
	}

	task, err := s.makeTask(
		ctx,
		req.GetProjectInfo(),
		req.GetWorkerId(),
		req.GetMasterId(),
		frameModel.WorkerType(req.GetTaskTypeId()),
		req.GetTaskConfig(),
		req.GetWorkerEpoch(),
	)
	if err != nil {
		// We use the code Aborted here per the suggestion in gRPC's documentation
		// "Use Aborted if the client should retry at a higher-level".
		// Failure to make task is usually a problem that the business logic
		// should be notified of.
		return nil, status.Error(codes.Aborted, err.Error())
	}

	if !s.taskCommitter.PreDispatchTask(req.GetRequestId(), task) {
		// The TaskCommitter failed to accept the task.
		// Currently, the only reason is duplicate requestID.
		return nil, status.Error(codes.AlreadyExists, "Duplicate request ID")
	}

	return &pb.PreDispatchTaskResponse{}, nil
}

// ConfirmDispatchTask implements Executor.ConfirmDispatchTask
func (s *Server) ConfirmDispatchTask(ctx context.Context, req *pb.ConfirmDispatchTaskRequest) (*pb.ConfirmDispatchTaskResponse, error) {
	if !s.isReadyToServe() {
		return nil, status.Error(codes.Unavailable, "executor server is not ready")
	}

	ok, err := s.taskCommitter.ConfirmDispatchTask(req.GetRequestId(), req.GetWorkerId())
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}
	if !ok {
		return nil, status.Error(codes.NotFound, "RequestID not found")
	}
	return &pb.ConfirmDispatchTaskResponse{}, nil
}

// Stop stops all running goroutines and releases resources in Server
func (s *Server) Stop() {
	if s.grpcSrv != nil {
		s.grpcSrv.Stop()
	}

	if s.tcpServer != nil {
		err := s.tcpServer.Close()
		if err != nil {
			log.L().Error("close tcp server", zap.Error(err))
		}
	}

	if s.metastores.IsInitialized() {
		s.metastores.Close()
	}

	if s.mockSrv != nil {
		s.mockSrv.Stop()
	}

	// TODO: unregister self from master.
}

func (s *Server) startForTest(ctx context.Context) (err error) {
	s.mockSrv, err = mock.NewExecutorServer(s.cfg.Addr, s)
	if err != nil {
		return err
	}

	err = s.initClients(ctx)
	if err != nil {
		return err
	}
	err = s.selfRegister(ctx)
	if err != nil {
		return err
	}
	go func() {
		err := s.keepHeartbeat(ctx)
		log.L().Info("heartbeat quits", zap.Error(err))
	}()
	return nil
}

func (s *Server) startMsgService(ctx context.Context, wg *errgroup.Group) (err error) {
	s.msgServer, err = p2p.NewDependentMessageRPCService(string(s.selfID), nil, s.grpcSrv)
	if err != nil {
		return err
	}
	wg.Go(func() error {
		// TODO refactor this
		return s.msgServer.Serve(ctx, nil)
	})
	return nil
}

func (s *Server) isReadyToServe() bool {
	return s.metastores.IsInitialized()
}

// Run drives server logic in independent background goroutines, and use error
// group to collect errors.
func (s *Server) Run(ctx context.Context) error {
	if test.GetGlobalTestFlag() {
		return s.startForTest(ctx)
	}

	wg, ctx := errgroup.WithContext(ctx)
	s.taskRunner = worker.NewTaskRunner(defaultRuntimeIncomingQueueLen, defaultRuntimeInitConcurrency)
	s.taskCommitter = worker.NewTaskCommitter(s.taskRunner, defaultTaskPreDispatchRequestTTL)
	defer func() {
		s.taskCommitter.Close()
	}()

	wg.Go(func() error {
		return s.taskRunner.Run(ctx)
	})

	wg.Go(func() error {
		taskStopReceiver := s.taskRunner.TaskStopReceiver()
		defer taskStopReceiver.Close()
		return s.jobAPISrv.listenStoppedJobs(ctx, taskStopReceiver.C)
	})

	err := s.initClients(ctx)
	if err != nil {
		return err
	}
	err = s.selfRegister(ctx)
	if err != nil {
		return err
	}

	if err := broker.PreCheckConfig(s.cfg.Storage); err != nil {
		return err
	}

	s.resourceBroker = broker.NewBroker(
		&s.cfg.Storage,
		s.selfID,
		s.masterClient)

	s.p2pMsgRouter = p2p.NewMessageRouter(p2p.NodeID(s.selfID), s.cfg.AdvertiseAddr)

	s.grpcSrv = grpc.NewServer()
	err = s.startMsgService(ctx, wg)
	if err != nil {
		return err
	}

	err = s.startTCPService(ctx, wg)
	if err != nil {
		return err
	}

	if err := s.metastores.Init(ctx, s.masterClient); err != nil {
		log.L().Error("Failed to init metastores", zap.Error(err))
		return err
	}

	discoveryAgent := discovery.NewAgent(s.masterClient, defaultDiscoveryAutoSyncInterval)
	wg.Go(func() error {
		return discoveryAgent.Run(ctx)
	})

	wg.Go(func() error {
		snap, receiver, err := discoveryAgent.Subscribe(ctx)
		if err != nil {
			return err
		}

		for _, node := range snap {
			log.Debug("update p2p msg router by snapshot", zap.Any("node", node))
			s.p2pMsgRouter.AddPeer(node.ID, node.Addr)
		}

		for {
			var event discovery.Event
			select {
			case <-ctx.Done():
				return perrors.Trace(err)
			case event = <-receiver.C:
			}

			log.Debug("update p2p msg router", zap.Any("event", event))
			if event.Tp == discovery.EventTypeDel {
				s.p2pMsgRouter.RemovePeer(event.Node.ID)
			} else if event.Tp == discovery.EventTypeAdd {
				s.p2pMsgRouter.AddPeer(event.Node.ID, event.Node.Addr)
			}
		}
	})

	wg.Go(func() error {
		snap, receiver, err := discoveryAgent.Subscribe(ctx)
		if err != nil {
			return err
		}
		defer receiver.Close()

		for _, node := range snap {
			if node.Tp != discovery.NodeTypeExecutor {
				continue
			}

			log.Debug("update executor client group by snapshot", zap.Any("node", node))
			err := s.executorGroup.AddExecutor(model.ExecutorID(node.ID), node.Addr)
			if err != nil {
				return err
			}
		}

		for {
			var event discovery.Event
			select {
			case <-ctx.Done():
				return perrors.Trace(err)
			case event = <-receiver.C:
			}

			if event.Node.Tp != discovery.NodeTypeExecutor {
				continue
			}

			log.Debug("update executor client group", zap.Any("event", event))
			if event.Tp == discovery.EventTypeDel {
				err := s.executorGroup.RemoveExecutor(model.ExecutorID(event.Node.ID))
				if err != nil {
					return err
				}
			} else if event.Tp == discovery.EventTypeAdd {
				err := s.executorGroup.AddExecutor(model.ExecutorID(event.Node.ID), event.Node.Addr)
				if err != nil {
					return err
				}
			}
		}
	})

	wg.Go(func() error {
		return s.keepHeartbeat(ctx)
	})

	wg.Go(func() error {
		return s.reportTaskResc(ctx)
	})

	wg.Go(func() error {
		return s.bgUpdateServerMasterClients(ctx)
	})

	wg.Go(func() error {
		return s.collectMetricLoop(ctx, defaultMetricInterval)
	})

	return wg.Wait()
}

// startTCPService starts grpc server and http server
func (s *Server) startTCPService(ctx context.Context, wg *errgroup.Group) error {
	tcpServer, err := tcpserver.NewTCPServer(s.cfg.Addr, &security.Credential{})
	if err != nil {
		return err
	}
	s.tcpServer = tcpServer
	pb.RegisterExecutorServiceServer(s.grpcSrv, s)
	pb.RegisterBrokerServiceServer(s.grpcSrv, s.resourceBroker)
	log.Info("listen address", zap.String("addr", s.cfg.Addr))

	wg.Go(func() error {
		return s.tcpServer.Run(ctx)
	})

	wg.Go(func() error {
		return s.grpcSrv.Serve(s.tcpServer.GrpcListener())
	})

	wg.Go(func() error {
		mux := http.NewServeMux()

		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		mux.Handle("/metrics", promutil.HTTPHandlerForMetric())
		mux.Handle(jobAPIPrefix, s.jobAPISrv)

		httpSrv := &http.Server{
			Handler: mux,
		}
		err := httpSrv.Serve(s.tcpServer.HTTP1Listener())
		if err != nil && !common.IsErrNetClosing(err) && err != http.ErrServerClosed {
			log.L().Error("http server returned", logutil.ShortError(err))
		}
		return err
	})
	return nil
}

func (s *Server) initClients(ctx context.Context) (err error) {
	// initServerMasterList is a MasterServerList with all servers marked as followers.
	initServerMasterList := getInitServerMasterList(s.cfg.Join)
	// TODO support TLS
	s.masterClient, err = pkgClient.NewServerMasterClientWithFailOver(initServerMasterList, nil)
	if err != nil {
		log.L().Info("master client init Failed",
			zap.String("server-addrs", s.cfg.Join),
			logutil.ShortError(err))
		return err
	}
	log.L().Info("master client init successful",
		zap.String("server-addrs", s.cfg.Join))

	s.executorGroup = pkgClient.NewExecutorGroup(nil, log.L())
	return nil
}

func (s *Server) selfRegister(ctx context.Context) error {
	registerReq := &pb.RegisterExecutorRequest{
		Executor: &pb.Executor{
			Name:       s.cfg.Name,
			Address:    s.cfg.AdvertiseAddr,
			Capability: defaultCapability,
		},
	}
	executorID, err := s.masterClient.RegisterExecutor(ctx, registerReq)
	if err != nil {
		return err
	}

	s.selfID = executorID
	log.L().Info("register successful", zap.String("executor-id", string(executorID)))
	return nil
}

// TODO: Right now heartbeat maintainable is too simple. We should look into
// what other frameworks do or whether we can use grpc heartbeat.
func (s *Server) keepHeartbeat(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.KeepAliveInterval)
	s.lastHearbeatTime = time.Now()
	defer func() {
		if test.GetGlobalTestFlag() {
			s.testCtx.NotifyExecutorChange(&test.ExecutorChangeEvent{
				Tp:   test.Delete,
				Time: time.Now(),
			})
		}
	}()
	rl := rate.NewLimiter(rate.Every(time.Second*5), 1 /*burst*/)
	for {
		select {
		case <-ctx.Done():
			return nil
		case t := <-ticker.C:
			if s.lastHearbeatTime.Add(s.cfg.KeepAliveTTL).Before(time.Now()) {
				return errors.ErrHeartbeat.GenWithStack("heartbeat timeout")
			}
			req := &pb.HeartbeatRequest{
				ExecutorId: string(s.selfID),
				Status:     int32(model.Running),
				Timestamp:  uint64(t.Unix()),
				// We set longer ttl for master, which is "ttl + rpc timeout", to avoid that
				// executor actually wait for a timeout when ttl is nearly up.
				Ttl: uint64(s.cfg.KeepAliveTTL.Milliseconds() + s.cfg.RPCTimeout.Milliseconds()),
			}
			resp, err := s.masterClient.Heartbeat(ctx, req)
			if err != nil {
				log.L().Error("heartbeat rpc meet error", zap.Error(err))
				if s.lastHearbeatTime.Add(s.cfg.KeepAliveTTL).Before(time.Now()) {
					return errors.WrapError(errors.ErrHeartbeat, err, "rpc")
				}
				continue
			}
			if resp.Err != nil {
				log.L().Warn("heartbeat response meet error", zap.Stringer("code", resp.Err.GetCode()))
				switch resp.Err.Code {
				case pb.ErrorCode_UnknownExecutor:
					log.L().Info("heartbeat failed, will retry", zap.Error(err))
					continue
				case pb.ErrorCode_TombstoneExecutor:
					return errors.ErrHeartbeat.GenWithStack("logic error: %s", resp.Err.GetMessage())
				case pb.ErrorCode_MasterNotReady:
					s.lastHearbeatTime = t
					if rl.Allow() {
						log.L().Info("heartbeat success with MasterNotReady")
					}
					continue
				default:
				}
			}
			// We aim to keep lastHbTime of executor consistent with lastHbTime of Master.
			// If we set the heartbeat time of executor to the start time of rpc, it will
			// be a little bit earlier than the heartbeat time of master, which is safe.
			// In contrast, if we set it to the end time of rpc, it might be a little bit
			// later than master's, which might cause that master wait for less time than executor.
			// This gap is unsafe.
			s.lastHearbeatTime = t
			if rl.Allow() {
				log.L().Info("heartbeat success",
					zap.String("leader", resp.Leader),
					zap.Strings("members", resp.Addrs))
			}
		}
	}
}

func getJoinURLs(addrs string) []string {
	return strings.Split(addrs, ",")
}

// getInitServerMasterList returns a MasterServerList with
// all servers marked as the follower.
func getInitServerMasterList(addrs string) pkgClient.MasterServerList {
	ret := make(pkgClient.MasterServerList, len(addrs))
	for _, addr := range getJoinURLs(addrs) {
		ret[addr] = false // Mark no leader
	}
	return ret
}

func (s *Server) reportTaskRescOnce(ctx context.Context) error {
	// TODO: do we need to report allocated resource to master?
	// TODO: Implement task-wise workload reporting in TaskRunner.
	/*
		rescs := s.workerRtm.Workload()
		req := &pb.ExecWorkloadRequest{
			// TODO: use which field as ExecutorId is more accurate
			ExecutorId: s.cfg.WorkerAddr,
			Workloads:  make([]*pb.ExecWorkload, 0, len(rescs)),
		}
		for tp, resc := range rescs {
			req.Workloads = append(req.Workloads, &pb.ExecWorkload{
				Tp:    pb.JobType(tp),
				Usage: int32(resc),
			})
		}
		resp, err := s.masterClient.ReportExecutorWorkload(ctx, req)
		if err != nil {
			return err
		}
		if resp.Err != nil {
			log.Warn("report executor workload error", zap.String("err", resp.Err.String()))
		}
	*/
	return nil
}

// reportTaskResc reports tasks resource usage to resource manager periodically
func (s *Server) reportTaskResc(ctx context.Context) error {
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			err := s.reportTaskRescOnce(ctx)
			if err != nil {
				return err
			}
		}
	}
}

func (s *Server) bgUpdateServerMasterClients(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return perrors.Trace(ctx.Err())
		case <-time.After(defaultDiscoveryAutoSyncInterval):
			masters, err := s.masterClient.ListMasters(ctx)
			if err != nil {
				log.Warn("update master list error", zap.Error(err))
				continue
			}
			masterList := make(pkgClient.MasterServerList)
			for _, m := range masters {
				masterList[m.Address] = m.IsLeader
			}
			if failoverCli, ok := s.masterClient.(*pkgClient.ServerMasterClientWithFailOver); ok {
				failoverCli.UpdateServerList(masterList)
			}
		}
	}
}

func (s *Server) collectMetricLoop(ctx context.Context, tickInterval time.Duration) error {
	metricRunningTask := executorTaskNumGauge.WithLabelValues("running")
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			metricRunningTask.Set(float64(s.taskRunner.TaskCount()))
		}
	}
}
