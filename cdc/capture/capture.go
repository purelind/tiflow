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

package capture

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pingcap/errors"
	"github.com/pingcap/failpoint"
	"github.com/pingcap/log"
	"github.com/pingcap/tiflow/cdc/model"
	"github.com/pingcap/tiflow/cdc/owner"
	"github.com/pingcap/tiflow/cdc/processor"
	"github.com/pingcap/tiflow/cdc/processor/pipeline/system"
	ssystem "github.com/pingcap/tiflow/cdc/sorter/db/system"
	"github.com/pingcap/tiflow/pkg/config"
	cdcContext "github.com/pingcap/tiflow/pkg/context"
	cerror "github.com/pingcap/tiflow/pkg/errors"
	"github.com/pingcap/tiflow/pkg/etcd"
	"github.com/pingcap/tiflow/pkg/migrate"
	"github.com/pingcap/tiflow/pkg/orchestrator"
	"github.com/pingcap/tiflow/pkg/p2p"
	"github.com/pingcap/tiflow/pkg/upstream"
	"github.com/pingcap/tiflow/pkg/util"
	"github.com/pingcap/tiflow/pkg/version"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.etcd.io/etcd/server/v3/mvcc"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// Capture represents a Capture server, it monitors the changefeed
// information in etcd and schedules Task on it.
type Capture interface {
	Run(ctx context.Context) error
	AsyncClose()
	Drain(ctx context.Context) <-chan struct{}
	Liveness() model.Liveness

	GetOwner() (owner.Owner, error)
	GetOwnerCaptureInfo(ctx context.Context) (*model.CaptureInfo, error)
	IsOwner() bool

	Info() (model.CaptureInfo, error)
	StatusProvider() owner.StatusProvider
	WriteDebugInfo(ctx context.Context, w io.Writer)

	GetUpstreamManager() (*upstream.Manager, error)
	GetEtcdClient() etcd.CDCEtcdClient
	// IsReady returns if the cdc server is ready
	// currently only check if ettcd data migration is done
	IsReady() bool
}

type captureImpl struct {
	// captureMu is used to protect the capture info and processorManager.
	captureMu        sync.Mutex
	info             *model.CaptureInfo
	processorManager processor.Manager
	liveness         model.Liveness
	config           *config.ServerConfig

	pdEndpoints     []string
	ownerMu         sync.Mutex
	owner           owner.Owner
	upstreamManager *upstream.Manager

	// session keeps alive between the capture and etcd
	session  *concurrency.Session
	election election

	EtcdClient       etcd.CDCEtcdClient
	sorterSystem     *ssystem.System
	tableActorSystem *system.System

	// MessageServer is the receiver of the messages from the other nodes.
	// It should be recreated each time the capture is restarted.
	MessageServer *p2p.MessageServer

	// MessageRouter manages the clients to send messages to all peers.
	MessageRouter p2p.MessageRouter

	// grpcService is a wrapper that can hold a MessageServer.
	// The instance should last for the whole life of the server,
	// regardless of server restarting.
	// This design is to solve the problem that grpc-go cannot gracefully
	// unregister a service.
	grpcService *p2p.ServerWrapper

	cancel context.CancelFunc

	migrator migrate.Migrator

	newProcessorManager func(
		captureInfo *model.CaptureInfo,
		upstreamManager *upstream.Manager,
		liveness *model.Liveness,
	) processor.Manager
	newOwner func(upstreamManager *upstream.Manager) owner.Owner
}

// NewCapture returns a new Capture instance
func NewCapture(pdEndpoints []string,
	etcdClient etcd.CDCEtcdClient,
	grpcService *p2p.ServerWrapper,
) Capture {
	conf := config.GetGlobalServerConfig()
	return &captureImpl{
		config:              config.GetGlobalServerConfig(),
		liveness:            model.LivenessCaptureAlive,
		EtcdClient:          etcdClient,
		grpcService:         grpcService,
		cancel:              func() {},
		pdEndpoints:         pdEndpoints,
		newProcessorManager: processor.NewManager,
		newOwner:            owner.NewOwner,

		migrator: migrate.NewMigrator(etcdClient, pdEndpoints, conf),
	}
}

// NewCapture4Test returns a new Capture instance for test.
func NewCapture4Test(o owner.Owner) *captureImpl {
	res := &captureImpl{
		info: &model.CaptureInfo{
			ID:            "capture-for-test",
			AdvertiseAddr: "127.0.0.1",
			Version:       "test",
		},
		migrator: &migrate.NoOpMigrator{},
		config:   config.GetGlobalServerConfig(),
	}
	res.owner = o
	return res
}

// NewCaptureWithManager4Test returns a new Capture instance for test.
func NewCaptureWithManager4Test(o owner.Owner, m *upstream.Manager) *captureImpl {
	res := &captureImpl{
		upstreamManager: m,
		migrator:        &migrate.NoOpMigrator{},
	}
	res.owner = o
	return res
}

// GetUpstreamManager is a Getter of capture's upstream manager
func (c *captureImpl) GetUpstreamManager() (*upstream.Manager, error) {
	if c.upstreamManager == nil {
		return nil, cerror.ErrUpstreamManagerNotReady
	}
	return c.upstreamManager, nil
}

func (c *captureImpl) GetEtcdClient() etcd.CDCEtcdClient {
	return c.EtcdClient
}

// reset the capture before run it.
func (c *captureImpl) reset(ctx context.Context) error {
	sess, err := concurrency.NewSession(
		c.EtcdClient.GetEtcdClient().Unwrap(),
		concurrency.WithTTL(c.config.CaptureSessionTTL))
	if err != nil {
		return cerror.WrapError(cerror.ErrNewCaptureFailed, err)
	}

	c.captureMu.Lock()
	defer c.captureMu.Unlock()
	c.info = &model.CaptureInfo{
		ID:            uuid.New().String(),
		AdvertiseAddr: c.config.AdvertiseAddr,
		Version:       version.ReleaseVersion,
	}

	if c.upstreamManager != nil {
		c.upstreamManager.Close()
	}
	c.upstreamManager = upstream.NewManager(ctx, c.EtcdClient.GetGCServiceID())
	_, err = c.upstreamManager.AddDefaultUpstream(c.pdEndpoints, c.config.Security)
	if err != nil {
		return cerror.WrapError(cerror.ErrNewCaptureFailed, err)
	}

	c.processorManager = c.newProcessorManager(c.info, c.upstreamManager, &c.liveness)
	if c.session != nil {
		// It can't be handled even after it fails, so we ignore it.
		_ = c.session.Close()
	}
	c.session = sess
	c.election = newElection(sess, etcd.CaptureOwnerKey(c.EtcdClient.GetClusterID()))

	if c.tableActorSystem != nil {
		c.tableActorSystem.Stop()
	}
	c.tableActorSystem = system.NewSystem()
	err = c.tableActorSystem.Start(ctx)
	if err != nil {
		return cerror.WrapError(cerror.ErrNewCaptureFailed, err)
	}
	if c.config.Debug.EnableDBSorter {
		if c.sorterSystem != nil {
			err := c.sorterSystem.Stop()
			if err != nil {
				log.Warn("stop sorter system failed", zap.Error(err))
			}
		}
		// Sorter dir has been set and checked when server starts.
		// See https://github.com/pingcap/tiflow/blob/9dad09/cdc/server.go#L275
		sortDir := config.GetGlobalServerConfig().Sorter.SortDir
		memPercentage := float64(config.GetGlobalServerConfig().Sorter.MaxMemoryPercentage) / 100
		c.sorterSystem = ssystem.NewSystem(sortDir, memPercentage, c.config.Debug.DB)
		err = c.sorterSystem.Start(ctx)
		if err != nil {
			return cerror.WrapError(cerror.ErrNewCaptureFailed, err)
		}
	}

	c.grpcService.Reset(nil)

	if c.MessageRouter != nil {
		c.MessageRouter.Close()
		c.MessageRouter.Wait()
		c.MessageRouter = nil
	}
	messageServerConfig := c.config.Debug.Messages.ToMessageServerConfig()
	c.MessageServer = p2p.NewMessageServer(c.info.ID, messageServerConfig)
	c.grpcService.Reset(c.MessageServer)

	messageClientConfig := c.config.Debug.Messages.ToMessageClientConfig()

	// Puts the advertise-addr of the local node to the client config.
	// This is for metrics purpose only, so that the receiver knows which
	// node the connections are from.
	advertiseAddr := c.config.AdvertiseAddr
	messageClientConfig.AdvertisedAddr = advertiseAddr

	c.MessageRouter = p2p.NewMessageRouter(c.info.ID, c.config.Security, messageClientConfig)

	log.Info("capture initialized", zap.Any("capture", c.info))
	return nil
}

// Run runs the capture
func (c *captureImpl) Run(ctx context.Context) error {
	defer log.Info("the capture routine has exited")
	// Limit the frequency of reset capture to avoid frequent recreating of resources
	rl := rate.NewLimiter(0.05, 2)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		ctx, cancel := context.WithCancel(ctx)
		c.cancel = cancel
		err := rl.Wait(ctx)
		if err != nil {
			if errors.Cause(err) == context.Canceled {
				return nil
			}
			return errors.Trace(err)
		}
		err = c.reset(ctx)
		if err != nil {
			log.Error("reset capture failed", zap.Error(err))
			return errors.Trace(err)
		}
		err = c.run(ctx)
		// if capture suicided, reset the capture and run again.
		// if the canceled error throw, there are two possible scenarios:
		//   1. the internal context canceled, it means some error happened in the internal, and the routine is exited, we should restart the capture
		//   2. the parent context canceled, it means that the caller of the capture hope the capture to exit, and this loop will return in the above `select` block
		// TODO: make sure the internal cancel should return the real error instead of context.Canceled
		if cerror.ErrCaptureSuicide.Equal(err) || context.Canceled == errors.Cause(err) {
			log.Info("capture recovered", zap.String("captureID", c.info.ID))
			continue
		}
		return errors.Trace(err)
	}
}

func (c *captureImpl) run(stdCtx context.Context) error {
	err := c.register(stdCtx)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.EtcdClient.DeleteCaptureInfo(timeoutCtx, c.info.ID); err != nil {
			log.Warn("failed to delete capture info when capture exited",
				zap.String("captureID", c.info.ID),
				zap.Error(err))
		}
		cancel()
	}()

	defer func() {
		c.AsyncClose()
		c.grpcService.Reset(nil)
	}()

	g, stdCtx := errgroup.WithContext(stdCtx)
	ctx := cdcContext.NewContext(stdCtx, &cdcContext.GlobalVars{
		CaptureInfo:      c.info,
		EtcdClient:       c.EtcdClient,
		TableActorSystem: c.tableActorSystem,
		SorterSystem:     c.sorterSystem,
		MessageServer:    c.MessageServer,
		MessageRouter:    c.MessageRouter,
	})

	g.Go(func() error {
		// when the campaignOwner returns an error, it means that the owner throws
		// an unrecoverable serious errors (recoverable errors are intercepted in the owner tick)
		// so we should also stop the owner and let capture restart or exit
		err := c.campaignOwner(ctx)
		log.Info("owner routine exited",
			zap.String("captureID", c.info.ID), zap.Error(err))
		return err
	})

	g.Go(func() error {
		processorFlushInterval := time.Duration(c.config.ProcessorFlushInterval)

		globalState := orchestrator.NewGlobalState(c.EtcdClient.GetClusterID())

		globalState.SetOnCaptureAdded(func(captureID model.CaptureID, addr string) {
			c.MessageRouter.AddPeer(captureID, addr)
		})
		globalState.SetOnCaptureRemoved(func(captureID model.CaptureID) {
			c.MessageRouter.RemovePeer(captureID)
		})

		// when the etcd worker of processor returns an error, it means that the processor throws an unrecoverable serious errors
		// (recoverable errors are intercepted in the processor tick)
		// so we should also stop the processor and let capture restart or exit
		err := c.runEtcdWorker(ctx, c.processorManager, globalState, processorFlushInterval, util.RoleProcessor.String())
		log.Info("processor routine exited",
			zap.String("captureID", c.info.ID), zap.Error(err))
		return err
	})

	g.Go(func() error {
		return c.MessageServer.Run(ctx)
	})

	return errors.Trace(g.Wait())
}

// Info gets the capture info
func (c *captureImpl) Info() (model.CaptureInfo, error) {
	c.captureMu.Lock()
	defer c.captureMu.Unlock()
	// when c.reset has not been called yet, c.info is nil.
	if c.info != nil {
		return *c.info, nil
	}
	return model.CaptureInfo{}, cerror.ErrCaptureNotInitialized.GenWithStackByArgs()
}

func (c *captureImpl) campaignOwner(ctx cdcContext.Context) error {
	// In most failure cases, we don't return error directly, just run another
	// campaign loop. We treat campaign loop as a special background routine.
	ownerFlushInterval := time.Duration(c.config.OwnerFlushInterval)
	failpoint.Inject("ownerFlushIntervalInject", func(val failpoint.Value) {
		ownerFlushInterval = time.Millisecond * time.Duration(val.(int))
	})
	// Limit the frequency of elections to avoid putting too much pressure on the etcd server
	rl := rate.NewLimiter(rate.Every(time.Second), 1 /* burst */)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		err := rl.Wait(ctx)
		if err != nil {
			if errors.Cause(err) == context.Canceled {
				return nil
			}
			return errors.Trace(err)
		}
		// Before campaign check liveness
		if c.liveness.Load() == model.LivenessCaptureStopping {
			// If the capture is stopping, do not campaign.
			log.Info("do not campaign owner, liveness is stopping")
			return nil
		}
		// Campaign to be the owner, it blocks until it been elected.
		if err := c.campaign(ctx); err != nil {
			switch errors.Cause(err) {
			case context.Canceled:
				return nil
			case mvcc.ErrCompacted:
				// the revision we requested is compacted, just retry
				continue
			}
			log.Warn("campaign owner failed",
				zap.String("captureID", c.info.ID), zap.Error(err))
			return cerror.ErrCaptureSuicide.GenWithStackByArgs()
		}
		// After campaign check liveness again.
		// It is possible it becomes the owner right after receiving SIGTERM.
		if c.liveness.Load() == model.LivenessCaptureStopping {
			// If the capture is stopping, resign actively.
			log.Info("resign owner actively, liveness is stopping")
			if resignErr := c.resign(ctx); resignErr != nil {
				return errors.Annotatef(resignErr, "resign owner failed, capture: %s", c.info.ID)
			}
			return nil
		}

		ownerRev, err := c.EtcdClient.GetOwnerRevision(ctx, c.info.ID)
		if err != nil {
			if errors.Cause(err) == context.Canceled {
				return nil
			}
			return errors.Trace(err)
		}

		// We do a copy of the globalVars here to avoid
		// accidental modifications and potential race conditions.
		globalVars := *ctx.GlobalVars()
		newGlobalVars := &globalVars
		newGlobalVars.OwnerRevision = ownerRev
		ownerCtx := cdcContext.NewContext(ctx, newGlobalVars)

		log.Info("campaign owner successfully",
			zap.String("captureID", c.info.ID),
			zap.Int64("ownerRev", ownerRev))

		owner := c.newOwner(c.upstreamManager)
		c.setOwner(owner)

		globalState := orchestrator.NewGlobalState(c.EtcdClient.GetClusterID())

		globalState.SetOnCaptureAdded(func(captureID model.CaptureID, addr string) {
			c.MessageRouter.AddPeer(captureID, addr)
		})
		globalState.SetOnCaptureRemoved(func(captureID model.CaptureID) {
			c.MessageRouter.RemovePeer(captureID)
		})

		err = c.runEtcdWorker(ownerCtx, owner,
			orchestrator.NewGlobalState(c.EtcdClient.GetClusterID()),
			ownerFlushInterval, util.RoleOwner.String())
		c.setOwner(nil)

		// if owner exits, resign the owner key,
		// use a new context to prevent the context from being cancelled.
		resignCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if resignErr := c.resign(resignCtx); resignErr != nil {
			if errors.Cause(resignErr) != context.DeadlineExceeded {
				log.Info("owner resign failed", zap.String("captureID", c.info.ID),
					zap.Error(resignErr), zap.Int64("ownerRev", ownerRev))
				cancel()
				return errors.Trace(resignErr)
			}

			log.Warn("owner resign timeout", zap.String("captureID", c.info.ID),
				zap.Error(resignErr), zap.Int64("ownerRev", ownerRev))
		}
		cancel()

		log.Info("owner resigned successfully",
			zap.String("captureID", c.info.ID), zap.Int64("ownerRev", ownerRev))
		if err != nil {
			log.Warn("run owner exited with error",
				zap.String("captureID", c.info.ID), zap.Int64("ownerRev", ownerRev),
				zap.Error(err))
			// for errors, return error and let capture exits or restart
			return errors.Trace(err)
		}
		// if owner exits normally, continue the campaign loop and try to election owner again
		log.Info("run owner exited normally",
			zap.String("captureID", c.info.ID), zap.Int64("ownerRev", ownerRev))
	}
}

func (c *captureImpl) runEtcdWorker(
	ctx cdcContext.Context,
	reactor orchestrator.Reactor,
	reactorState orchestrator.ReactorState,
	timerInterval time.Duration,
	role string,
) error {
	etcdWorker, err := orchestrator.NewEtcdWorker(c.EtcdClient,
		etcd.BaseKey(c.EtcdClient.GetClusterID()), reactor, reactorState, c.migrator)
	if err != nil {
		return errors.Trace(err)
	}
	if err := etcdWorker.Run(ctx, c.session, timerInterval, role); err != nil {
		// We check ttl of lease instead of check `session.Done`, because
		// `session.Done` is only notified when etcd client establish a
		// new keepalive request, there could be a time window as long as
		// 1/3 of session ttl that `session.Done` can't be triggered even
		// the lease is already revoked.
		switch {
		case cerror.ErrEtcdSessionDone.Equal(err),
			cerror.ErrLeaseExpired.Equal(err):
			log.Warn("session is disconnected", zap.Error(err))
			return cerror.ErrCaptureSuicide.GenWithStackByArgs()
		}
		lease, inErr := c.EtcdClient.GetEtcdClient().TimeToLive(ctx, c.session.Lease())
		if inErr != nil {
			return cerror.WrapError(cerror.ErrPDEtcdAPIError, inErr)
		}
		if lease.TTL == int64(-1) {
			log.Warn("session is disconnected", zap.Error(err))
			return cerror.ErrCaptureSuicide.GenWithStackByArgs()
		}
		return errors.Trace(err)
	}
	return nil
}

func (c *captureImpl) setOwner(owner owner.Owner) {
	c.ownerMu.Lock()
	defer c.ownerMu.Unlock()
	c.owner = owner
}

// GetOwner returns owner if it is the owner.
func (c *captureImpl) GetOwner() (owner.Owner, error) {
	c.ownerMu.Lock()
	defer c.ownerMu.Unlock()
	if c.owner == nil {
		return nil, cerror.ErrNotOwner.GenWithStackByArgs()
	}
	return c.owner, nil
}

// campaign to be an owner.
func (c *captureImpl) campaign(ctx context.Context) error {
	failpoint.Inject("capture-campaign-compacted-error", func() {
		failpoint.Return(errors.Trace(mvcc.ErrCompacted))
	})
	return cerror.WrapError(cerror.ErrCaptureCampaignOwner, c.election.campaign(ctx, c.info.ID))
}

// resign lets an owner start a new election.
func (c *captureImpl) resign(ctx context.Context) error {
	failpoint.Inject("capture-resign-failed", func() {
		failpoint.Return(errors.New("capture resign failed"))
	})
	return cerror.WrapError(cerror.ErrCaptureResignOwner, c.election.resign(ctx))
}

// register the capture by put the capture's information in etcd
func (c *captureImpl) register(ctx context.Context) error {
	err := c.EtcdClient.PutCaptureInfo(ctx, c.info, c.session.Lease())
	if err != nil {
		return cerror.WrapError(cerror.ErrCaptureRegister, err)
	}
	return nil
}

// AsyncClose closes the capture by deregister it from etcd
// Note: this function should be reentrant
func (c *captureImpl) AsyncClose() {
	defer c.cancel()
	// Safety: Here we mainly want to stop the owner
	// and ignore it if the owner does not exist or is not set.
	o, _ := c.GetOwner()
	if o != nil {
		o.AsyncStop()
		log.Info("owner closed", zap.String("captureID", c.info.ID))
	}

	c.captureMu.Lock()
	defer c.captureMu.Unlock()
	if c.processorManager != nil {
		c.processorManager.AsyncClose()
	}
	log.Info("processor manager closed", zap.String("captureID", c.info.ID))

	if c.tableActorSystem != nil {
		c.tableActorSystem.Stop()
		c.tableActorSystem = nil
	}
	log.Info("table actor system closed", zap.String("captureID", c.info.ID))

	if c.sorterSystem != nil {
		err := c.sorterSystem.Stop()
		if err != nil {
			log.Warn("stop sorter system failed",
				zap.String("captureID", c.info.ID), zap.Error(err))
		}
		c.sorterSystem = nil
	}
	log.Info("sorter actor system closed", zap.String("captureID", c.info.ID))

	c.grpcService.Reset(nil)
	if c.MessageRouter != nil {
		c.MessageRouter.Close()
		c.MessageRouter.Wait()
		c.MessageRouter = nil
	}
	log.Info("message router closed", zap.String("captureID", c.info.ID))
}

// Drain removes tables in the current TiCDC instance.
func (c *captureImpl) Drain(ctx context.Context) <-chan struct{} {
	log.Info("draining capture, removing all tables on the capture",
		zap.String("captureID", c.info.ID))

	const drainInterval = 100 * time.Millisecond
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(drainInterval)
		defer ticker.Stop()
		for {
			complete := c.drainImpl(ctx)
			if complete {
				return
			}
			ticker.Reset(drainInterval)
			select {
			case <-ctx.Done():
				// Give up when the context cancels. In the current
				// implementation, it is caused TiCDC receives a second signal
				// and begins force shutdown.
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

func (c *captureImpl) drainImpl(ctx context.Context) bool {
	if !c.config.Debug.EnableSchedulerV3 {
		// Skip drain as two phase scheduler is disabled.
		return true
	}

	// Step 1, resign ownership.
	o, _ := c.GetOwner()
	if o != nil {
		doneCh := make(chan error, 1)
		query := &owner.Query{Tp: owner.QueryCaptures, Data: []*model.CaptureInfo{}}
		o.Query(query, doneCh)
		select {
		case <-ctx.Done():
		case err := <-doneCh:
			if err != nil {
				log.Warn("query capture count failed, retry", zap.Error(err))
				return false
			}
		}
		if len(query.Data.([]*model.CaptureInfo)) <= 1 {
			// There is only one capture, the owner itself. It's impossible to
			// resign owner nor move out tables, give up drain.
			log.Warn("there is only one capture, skip drain")
			return true
		}
		o.AsyncStop()
		// Make sure it's not the owner before step 2.
		return false
	}
	// Step 2, wait for moving out all tables.
	// Set liveness stopping, owners will move all tables out in the capture.
	c.liveness.Store(model.LivenessCaptureStopping)
	// Check if there is an owner.
	_, err := c.GetEtcdClient().GetOwnerID(ctx)
	if err != nil {
		if errors.Cause(err) != concurrency.ErrElectionNoLeader {
			log.Error("fail to get owner ID, retry")
			return false
		}
		// There is no owner. It's impossible to move tables out, give up drain.
		log.Warn("there is no owner, skip drain")
		return true
	}

	queryDone := make(chan error, 1)
	tableCh := make(chan int, 1)
	c.processorManager.QueryTableCount(ctx, tableCh, queryDone)
	select {
	case <-ctx.Done():
	case err := <-queryDone:
		if err != nil {
			log.Warn("query table count failed, retry", zap.Error(err))
			return false
		}
	}
	select {
	case <-ctx.Done():
	case tableCount := <-tableCh:
		if tableCount == 0 {
			log.Info("all tables removed, drain capture complete")
			return true
		}
	}
	return false
}

// Liveness returns liveness of the capture.
func (c *captureImpl) Liveness() model.Liveness {
	return c.liveness.Load()
}

// WriteDebugInfo writes the debug info into writer.
func (c *captureImpl) WriteDebugInfo(ctx context.Context, w io.Writer) {
	wait := func(done <-chan error) {
		var err error
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case err = <-done:
		}
		if err != nil {
			log.Warn("write debug info failed", zap.Error(err))
		}
	}
	// Safety: Because we are mainly outputting information about the owner here,
	// if the owner does not exist or is not set, the information will not be output.
	o, _ := c.GetOwner()
	if o != nil {
		doneOwner := make(chan error, 1)
		fmt.Fprintf(w, "\n\n*** owner info ***:\n\n")
		o.WriteDebugInfo(w, doneOwner)
		// wait the debug info printed
		wait(doneOwner)
	}

	doneM := make(chan error, 1)
	c.captureMu.Lock()
	if c.processorManager != nil {
		fmt.Fprintf(w, "\n\n*** processors info ***:\n\n")
		c.processorManager.WriteDebugInfo(ctx, w, doneM)
	}
	// NOTICE: we must release the lock before wait the debug info process down.
	// Otherwise, the capture initialization and request response will compete
	// for captureMu resulting in a deadlock.
	c.captureMu.Unlock()
	// wait the debug info printed
	wait(doneM)
}

// IsOwner returns whether the capture is an owner
func (c *captureImpl) IsOwner() bool {
	c.ownerMu.Lock()
	defer c.ownerMu.Unlock()
	return c.owner != nil
}

// GetOwnerCaptureInfo return the owner capture info of current TiCDC cluster
func (c *captureImpl) GetOwnerCaptureInfo(ctx context.Context) (*model.CaptureInfo, error) {
	_, captureInfos, err := c.EtcdClient.GetCaptures(ctx)
	if err != nil {
		return nil, err
	}

	ownerID, err := c.EtcdClient.GetOwnerID(ctx)
	if err != nil {
		return nil, err
	}

	for _, captureInfo := range captureInfos {
		if captureInfo.ID == ownerID {
			return captureInfo, nil
		}
	}
	return nil, cerror.ErrOwnerNotFound.FastGenByArgs()
}

// StatusProvider returns owner's StatusProvider.
func (c *captureImpl) StatusProvider() owner.StatusProvider {
	c.ownerMu.Lock()
	defer c.ownerMu.Unlock()
	if c.owner == nil {
		return nil
	}
	return owner.NewStatusProvider(c.owner)
}

func (c *captureImpl) IsReady() bool {
	return c.migrator.IsMigrateDone()
}
