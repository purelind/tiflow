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

package txn

import (
	"context"
	"net/url"
	"sync/atomic"

	"github.com/pingcap/errors"
	"github.com/pingcap/tiflow/cdc/model"
	"github.com/pingcap/tiflow/cdc/sinkv2/eventsink"
	"github.com/pingcap/tiflow/cdc/sinkv2/eventsink/txn/mysql"
	"github.com/pingcap/tiflow/cdc/sinkv2/metrics"
	"github.com/pingcap/tiflow/pkg/causality"
	"github.com/pingcap/tiflow/pkg/config"
	psink "github.com/pingcap/tiflow/pkg/sink"
	pmysql "github.com/pingcap/tiflow/pkg/sink/mysql"
)

const (
	// DefaultConflictDetectorSlots indicates the default slot count of conflict detector.
	DefaultConflictDetectorSlots int64 = 1024 * 1024
)

// Assert EventSink[E event.TableEvent] implementation
var _ eventsink.EventSink[*model.SingleTableTxn] = (*sink)(nil)

// sink is the sink for SingleTableTxn.
type sink struct {
	conflictDetector *causality.ConflictDetector[*worker, *txnEvent]
	workers          []*worker
	cancel           func()

	// set when the sink is closed explicitly. and then subsequence `WriteEvents` call
	// should returns an error.
	closed int32
}

func newSink(ctx context.Context, backends []backend, errCh chan<- error, conflictDetectorSlots int64) *sink {
	workers := make([]*worker, 0, len(backends))
	for i, backend := range backends {
		w := newWorker(ctx, i, backend, errCh)
		w.runBackgroundLoop()
		workers = append(workers, w)
	}
	detector := causality.NewConflictDetector[*worker, *txnEvent](workers, conflictDetectorSlots)
	return &sink{conflictDetector: detector, workers: workers}
}

// NewMySQLSink creates a mysql sink with given parameters.
func NewMySQLSink(
	ctx context.Context,
	sinkURI *url.URL,
	replicaConfig *config.ReplicaConfig,
	errCh chan<- error,
	conflictDetectorSlots int64,
) (*sink, error) {
	var getConn pmysql.Factory = pmysql.CreateMySQLDBConn

	ctx1, cancel := context.WithCancel(ctx)
	statistics := metrics.NewStatistics(ctx1, psink.TxnSink)
	backendImpls, err := mysql.NewMySQLBackends(ctx, sinkURI, replicaConfig, getConn, statistics)
	if err != nil {
		cancel()
		return nil, err
	}

	backends := make([]backend, 0, len(backendImpls))
	for _, impl := range backendImpls {
		backends = append(backends, impl)
	}
	sink := newSink(ctx, backends, errCh, conflictDetectorSlots)
	sink.cancel = cancel

	return sink, nil
}

// WriteEvents writes events to the sink.
func (s *sink) WriteEvents(rows ...*eventsink.TxnCallbackableEvent) error {
	if atomic.LoadInt32(&s.closed) != 0 {
		return errors.Trace(errors.New("closed sink"))
	}
	for _, row := range rows {
		err := s.conflictDetector.Add(newTxnEvent(row))
		if err != nil {
			return err
		}
	}
	return nil
}

// Close closes the sink. It won't wait for all pending items backend handled.
func (s *sink) Close() error {
	atomic.StoreInt32(&s.closed, 1)
	s.conflictDetector.Close()
	for _, w := range s.workers {
		w.Close()
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	return nil
}
