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

package owner

import (
	"github.com/pingcap/errors"
	"github.com/pingcap/log"
	tidbkv "github.com/pingcap/tidb/kv"
	timeta "github.com/pingcap/tidb/meta"
	timodel "github.com/pingcap/tidb/parser/model"
	"github.com/pingcap/tiflow/cdc/entry/schema"
	"github.com/pingcap/tiflow/cdc/kv"
	"github.com/pingcap/tiflow/cdc/model"
	"github.com/pingcap/tiflow/pkg/config"
	cerror "github.com/pingcap/tiflow/pkg/errors"
	"github.com/pingcap/tiflow/pkg/filter"
	"github.com/pingcap/tiflow/pkg/util"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type schemaWrap4Owner struct {
	schemaSnapshot              *schema.Snapshot
	filter                      filter.Filter
	config                      *config.ReplicaConfig
	allPhysicalTablesCache      []model.TableID
	ddlHandledTs                model.Ts
	schemaVersion               int64
	id                          model.ChangeFeedID
	metricIgnoreDDLEventCounter prometheus.Counter
}

func newSchemaWrap4Owner(
	kvStorage tidbkv.Storage, startTs model.Ts,
	config *config.ReplicaConfig, id model.ChangeFeedID,
) (*schemaWrap4Owner, error) {
	var (
		meta    *timeta.Meta
		version int64
	)
	if kvStorage != nil {
		var err error
		meta, err = kv.GetSnapshotMeta(kvStorage, startTs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		version, err = schema.GetSchemaVersion(meta)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	schemaSnap, err := schema.NewSingleSnapshotFromMeta(meta, startTs, config.ForceReplicate)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// It is no matter to use an empty as timezone here because schemaWrap4Owner
	// doesn't use expression filter's method.
	f, err := filter.NewFilter(config, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &schemaWrap4Owner{
		schemaSnapshot: schemaSnap,
		filter:         f,
		config:         config,
		ddlHandledTs:   startTs,
		schemaVersion:  version,
		id:             id,
		metricIgnoreDDLEventCounter: changefeedIgnoredDDLEventCounter.
			WithLabelValues(id.Namespace, id.ID),
	}, nil
}

// AllPhysicalTables returns the table IDs of all tables and partition tables.
func (s *schemaWrap4Owner) AllPhysicalTables() []model.TableID {
	if s.allPhysicalTablesCache != nil {
		return s.allPhysicalTablesCache
	}
	// NOTE: it's better to pre-allocate the vector. However in the current implementation
	// we can't know how many valid tables in the snapshot.
	s.allPhysicalTablesCache = make([]model.TableID, 0)
	s.schemaSnapshot.IterTables(true, func(tblInfo *model.TableInfo) {
		if s.shouldIgnoreTable(tblInfo) {
			return
		}
		if pi := tblInfo.GetPartitionInfo(); pi != nil {
			for _, partition := range pi.Definitions {
				s.allPhysicalTablesCache = append(s.allPhysicalTablesCache, partition.ID)
			}
		} else {
			s.allPhysicalTablesCache = append(s.allPhysicalTablesCache, tblInfo.ID)
		}
	})
	return s.allPhysicalTablesCache
}

// AllTableNames returns the table names of all tables that are being replicated.
func (s *schemaWrap4Owner) AllTableNames() []model.TableName {
	names := make([]model.TableName, 0, len(s.allPhysicalTablesCache))
	s.schemaSnapshot.IterTables(true, func(tblInfo *model.TableInfo) {
		if !s.shouldIgnoreTable(tblInfo) {
			names = append(names, tblInfo.TableName)
		}
	})
	return names
}

func (s *schemaWrap4Owner) HandleDDL(job *timodel.Job) error {
	// We use schemaVersion to check if an already-executed DDL job is processed for a second time.
	// Unexecuted DDL jobs should have largest schemaVersions
	if job.BinlogInfo.FinishedTS <= s.ddlHandledTs || job.BinlogInfo.SchemaVersion <= s.schemaVersion {
		log.Warn("job finishTs is less than schema handleTs, discard invalid job",
			zap.String("namespace", s.id.Namespace),
			zap.String("changefeed", s.id.ID),
			zap.Stringer("job", job),
			zap.Any("ddlHandledTs", s.ddlHandledTs),
			zap.Int64("schemaVersion", s.schemaVersion),
			zap.Int64("jobSchemaVersion", job.BinlogInfo.SchemaVersion),
		)
		return nil
	}
	s.allPhysicalTablesCache = nil
	err := s.schemaSnapshot.HandleDDL(job)
	if err != nil {
		log.Error("owner update schema failed",
			zap.String("namespace", s.id.Namespace),
			zap.String("changefeed", s.id.ID),
			zap.String("DDL", job.Query),
			zap.Stringer("job", job),
			zap.Error(err),
			zap.Any("role", util.RoleOwner))
		return errors.Trace(err)
	}
	log.Info("owner update schema snapshot",
		zap.String("namespace", s.id.Namespace),
		zap.String("changefeed", s.id.ID),
		zap.String("DDL", job.Query),
		zap.Stringer("job", job),
		zap.Any("role", util.RoleOwner))

	s.ddlHandledTs = job.BinlogInfo.FinishedTS
	s.schemaVersion = job.BinlogInfo.SchemaVersion
	return nil
}

func (s *schemaWrap4Owner) IsIneligibleTableID(tableID model.TableID) bool {
	return s.schemaSnapshot.IsIneligibleTableID(tableID)
}

// parseRenameTables gets a list of DDLEvent from a rename tables DDL job.
func (s *schemaWrap4Owner) parseRenameTables(
	job *timodel.Job,
) ([]*model.DDLEvent, error) {
	var (
		oldSchemaIDs, newSchemaIDs, oldTableIDs []int64
		newTableNames, oldSchemaNames           []*timodel.CIStr
		ddlEvents                               []*model.DDLEvent
	)

	err := job.DecodeArgs(&oldSchemaIDs, &newSchemaIDs,
		&newTableNames, &oldTableIDs, &oldSchemaNames)
	if err != nil {
		return nil, errors.Trace(err)
	}

	multiTableInfos := job.BinlogInfo.MultipleTableInfos
	if len(multiTableInfos) != len(oldSchemaIDs) ||
		len(multiTableInfos) != len(newSchemaIDs) ||
		len(multiTableInfos) != len(newTableNames) ||
		len(multiTableInfos) != len(oldTableIDs) ||
		len(multiTableInfos) != len(oldSchemaNames) {
		return nil, cerror.ErrInvalidDDLJob.GenWithStackByArgs(job.ID)
	}

	for i, tableInfo := range multiTableInfos {
		newSchema, ok := s.schemaSnapshot.SchemaByID(newSchemaIDs[i])
		if !ok {
			return nil, cerror.ErrSnapshotSchemaNotFound.GenWithStackByArgs(
				newSchemaIDs[i])
		}
		newSchemaName := newSchema.Name.L
		oldSchemaName := oldSchemaNames[i].L
		event := new(model.DDLEvent)
		preTableInfo, ok := s.schemaSnapshot.PhysicalTableByID(tableInfo.ID)
		if !ok {
			return nil, cerror.ErrSchemaStorageTableMiss.GenWithStackByArgs(
				job.TableID)
		}

		event.FromRenameTablesJob(job, oldSchemaName,
			newSchemaName, preTableInfo, tableInfo)
		ddlEvents = append(ddlEvents, event)
	}

	return ddlEvents, nil
}

// BuildDDLEvents builds ddl events from a DDL job.
// The result contains more than one DDLEvent for a rename tables job.
// Note: If BuildDDLEvents return (nil, nil), it means the DDL Job should be ignored.
func (s *schemaWrap4Owner) BuildDDLEvents(
	job *timodel.Job,
) ([]*model.DDLEvent, error) {
	var err error
	var preTableInfo *model.TableInfo
	ddlEvents := make([]*model.DDLEvent, 0)
	switch job.Type {
	case timodel.ActionRenameTables:
		ddlEvents, err = s.parseRenameTables(job)
		if err != nil {
			return nil, errors.Trace(err)
		}
	default:
		event := new(model.DDLEvent)
		preTableInfo, err = s.schemaSnapshot.PreTableInfo(job)
		if err != nil {
			log.Error("build DDL event fail",
				zap.Any("job", job), zap.Error(err))
			return nil, errors.Trace(err)
		}
		err = s.schemaSnapshot.FillSchemaName(job)
		if err != nil {
			return nil, errors.Trace(err)
		}
		event.FromJob(job, preTableInfo)
		ddlEvents = append(ddlEvents, event)
	}
	// filter out ddl here
	res := make([]*model.DDLEvent, 0, len(ddlEvents))
	for _, event := range ddlEvents {
		ignored, err := s.filter.ShouldIgnoreDDLEvent(event)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if ignored {
			s.metricIgnoreDDLEventCounter.Inc()
			log.Info(
				"DDL event ignored",
				zap.String("namespace", s.id.Namespace),
				zap.String("changefeed", s.id.ID),
				zap.String("query", event.Query),
				zap.Uint64("startTs", event.StartTs),
				zap.Uint64("commitTs", event.CommitTs),
			)
			continue
		}
		res = append(res, event)
	}
	return res, nil
}

func (s *schemaWrap4Owner) shouldIgnoreTable(t *model.TableInfo) bool {
	schemaName := t.TableName.Schema
	tableName := t.TableName.Table
	if s.filter.ShouldIgnoreTable(schemaName, tableName) {
		return true
	}
	if !t.IsEligible(s.config.ForceReplicate) {
		// Sequence is not supported yet, and always ineligible.
		// Skip Warn to avoid confusion.
		// See https://github.com/pingcap/tiflow/issues/4559
		if !t.IsSequence() {
			log.Warn("skip ineligible table",
				zap.String("namespace", s.id.Namespace),
				zap.String("changefeed", s.id.ID),
				zap.Int64("tableID", t.ID),
				zap.Stringer("tableName", t.TableName),
			)
		}
		return true
	}
	return false
}
