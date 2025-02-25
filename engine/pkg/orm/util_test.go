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

package orm

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	metaMock "github.com/pingcap/tiflow/engine/pkg/meta/mock"
	cerrors "github.com/pingcap/tiflow/pkg/errors"
)

func TestIsNotFoundError(t *testing.T) {
	b := IsNotFoundError(cerrors.ErrMetaEntryNotFound.GenWithStackByArgs("error"))
	require.True(t, b)

	b = IsNotFoundError(cerrors.ErrMetaEntryNotFound.GenWithStack("err:%s", "error"))
	require.True(t, b)

	b = IsNotFoundError(cerrors.ErrMetaEntryNotFound.Wrap(errors.New("error")))
	require.True(t, b)

	b = IsNotFoundError(cerrors.ErrMetaNewClientFail.Wrap(errors.New("error")))
	require.False(t, b)

	b = IsNotFoundError(errors.New("error"))
	require.False(t, b)
}

// nolint: deadcode
// TODO: The reason why index sequence is unstable is unknown
func testInitialize(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.Nil(t, err)
	defer db.Close()
	defer mock.ExpectClose()

	// common execution for orm
	mock.ExpectQuery("SELECT VERSION()").
		WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("5.7.35-log"))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC limit 1")).WillReturnRows(
		sqlmock.NewRows([]string{"SCHEMA_NAME"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `project_infos` (`seq_id` bigint unsigned AUTO_INCREMENT," +
		"`created_at` datetime(3) NULL,`updated_at` datetime(3) NULL," +
		"`id` varchar(128) not null,`name` varchar(128) not null,PRIMARY KEY (`seq_id`)," +
		"UNIQUE INDEX `uidx_id` (`id`))")).WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC limit 1")).WillReturnRows(
		sqlmock.NewRows([]string{"SCHEMA_NAME"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `project_operations` (`seq_id` bigint unsigned AUTO_INCREMENT," +
		"`project_id` varchar(128) not null,`operation` varchar(16) not null,`job_id` varchar(128) not null," +
		"`created_at` datetime(3) NULL,PRIMARY KEY (`seq_id`),INDEX `idx_op` (`project_id`,`created_at`))")).WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC limit 1")).WillReturnRows(
		sqlmock.NewRows([]string{"SCHEMA_NAME"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `master_meta_kv_data` (`seq_id` bigint unsigned AUTO_INCREMENT,`created_at` datetime(3) NULL," +
		"`updated_at` datetime(3) NULL,`project_id` varchar(128) not null,`id` varchar(128) not null,`type` smallint not null COMMENT 'JobManager(1),CvsJobMaster(2),FakeJobMaster(3),DMJobMaster(4),CDCJobMaster(5)'," +
		"`status` tinyint not null COMMENT 'Uninit(1),Init(2),Finished(3),Stopped(4)',`node_id` varchar(128) not null,`address` varchar(256) not null,`epoch` bigint not null," +
		"`config` blob,`extend_message` text,`deleted` datetime(3) NULL,PRIMARY KEY (`seq_id`),INDEX `idx_mst` (`project_id`,`status`),UNIQUE INDEX `uidx_mid` (`id`))")).WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC limit 1")).WillReturnRows(
		sqlmock.NewRows([]string{"SCHEMA_NAME"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `worker_statuses` (`seq_id` bigint unsigned AUTO_INCREMENT,`created_at` datetime(3) NULL," +
		"`updated_at` datetime(3) NULL,`project_id` varchar(128) not null,`job_id` varchar(128) not null,`id` varchar(128) not null,`type` smallint not null COMMENT 'JobManager(1),CvsJobMaster(2),FakeJobMaster(3),DMJobMaster(4)," +
		"CDCJobMaster(5),CvsTask(6),FakeTask(7),DMTask(8),CDCTask(9),WorkerDMDump(10),WorkerDMLoad(11),WorkerDMSync(12)',`status` tinyint not null COMMENT 'Normal(1),Created(2),Init(3),Error(4),Finished(5),Stopped(6)'," +
		"`epoch` bigint not null,`errmsg` text,`extend_bytes` blob,PRIMARY KEY (`seq_id`),UNIQUE INDEX `uidx_wid` (`job_id`,`id`),INDEX `idx_wst` (`job_id`,`status`))")).WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC limit 1")).WillReturnRows(
		sqlmock.NewRows([]string{"SCHEMA_NAME"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `resource_meta` (`seq_id` bigint unsigned AUTO_INCREMENT,`created_at` datetime(3) NULL,`updated_at` datetime(3) NULL," +
		"`project_id` varchar(128) not null,`id` varchar(128) not null,`job_id` varchar(128) not null,`worker_id` varchar(128) not null,`executor_id` varchar(128) not null," +
		"`gc_pending` BOOLEAN,`deleted` BOOLEAN,PRIMARY KEY (`seq_id`),UNIQUE INDEX `uidx_rid` (`job_id`,`id`),INDEX `idx_rei` (`executor_id`,`id`))")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC limit 1")).
		WillReturnRows(sqlmock.NewRows([]string{"SCHEMA_NAME"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `logic_epoches` (`seq_id` bigint unsigned AUTO_INCREMENT,`created_at` datetime(3) NULL," +
		"`updated_at` datetime(3) NULL,`job_id` varchar(128) not null,`epoch` bigint not null default 1,PRIMARY KEY (`seq_id`),UNIQUE INDEX `uidx_jk` (`job_id`))")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA " +
		"where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC limit 1")).WillReturnRows(
		sqlmock.NewRows([]string{"SCHEMA_NAME"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `logic_epoches` (`seq_id` bigint unsigned AUTO_INCREMENT," +
		"`created_at` datetime(3) NULL,`updated_at` datetime(3) NULL,`job_id` varchar(128) not null,`epoch` bigint not null default 1," +
		"PRIMARY KEY (`seq_id`),UNIQUE INDEX `uidx_jk` (`job_id`))")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	conn := metaMock.NewClientConnWithDB(db)
	require.NotNil(t, conn)
	defer conn.Close()

	err = InitAllFrameworkModels(context.TODO(), conn)
	require.Nil(t, err)
}

func TestInitEpochModel(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.Nil(t, err)
	defer db.Close()
	defer mock.ExpectClose()

	ctx, cancel := context.WithTimeout(context.TODO(), 1*time.Second)
	defer cancel()

	err = InitEpochModel(ctx, nil)
	require.Regexp(t, regexp.QuoteMeta("input client conn is nil"), err.Error())

	mock.ExpectQuery("SELECT VERSION()").
		WillReturnRows(sqlmock.NewRows([]string{"VERSION()"}).AddRow("5.7.35-log"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT SCHEMA_NAME from Information_schema.SCHEMATA " +
		"where SCHEMA_NAME LIKE ? ORDER BY SCHEMA_NAME=? DESC limit 1")).WillReturnRows(
		sqlmock.NewRows([]string{"SCHEMA_NAME"}))
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `logic_epoches` (`seq_id` bigint unsigned AUTO_INCREMENT," +
		"`created_at` datetime(3) NULL,`updated_at` datetime(3) NULL,`job_id` varchar(128) not null,`epoch` bigint not null default 1," +
		"PRIMARY KEY (`seq_id`),UNIQUE INDEX `uidx_jk` (`job_id`))")).
		WillReturnResult(sqlmock.NewResult(1, 1))

	conn := metaMock.NewClientConnWithDB(db)
	require.NotNil(t, conn)
	defer conn.Close()

	err = InitEpochModel(ctx, conn)
	require.NoError(t, err)
}
