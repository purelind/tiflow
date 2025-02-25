//  Copyright 2021 PingCAP, Inc.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  See the License for the specific language governing permissions and
//  limitations under the License.

package writer

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/golang/mock/gomock"
	"github.com/pingcap/errors"
	"github.com/pingcap/log"
	mockstorage "github.com/pingcap/tidb/br/pkg/mock/storage"
	"github.com/pingcap/tidb/br/pkg/storage"
	"github.com/pingcap/tiflow/cdc/model"
	"github.com/pingcap/tiflow/cdc/redo/common"
	cerror "github.com/pingcap/tiflow/pkg/errors"
	"github.com/pingcap/tiflow/pkg/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

func TestLogWriterWriteLog(t *testing.T) {
	type arg struct {
		ctx     context.Context
		tableID int64
		rows    []*model.RedoRowChangedEvent
	}
	tests := []struct {
		name      string
		args      arg
		wantTs    uint64
		isRunning bool
		writerErr error
		wantErr   error
	}{
		{
			name: "happy",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				rows: []*model.RedoRowChangedEvent{
					{
						Row: &model.RowChangedEvent{
							Table: &model.TableName{TableID: 111}, CommitTs: 1,
						},
					},
				},
			},
			isRunning: true,
			writerErr: nil,
		},
		{
			name: "writer err",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				rows: []*model.RedoRowChangedEvent{
					{Row: nil},
					{
						Row: &model.RowChangedEvent{
							Table: &model.TableName{TableID: 11}, CommitTs: 11,
						},
					},
				},
			},
			writerErr: errors.New("err"),
			wantErr:   errors.New("err"),
			isRunning: true,
		},
		{
			name: "len(rows)==0",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				rows:    []*model.RedoRowChangedEvent{},
			},
			writerErr: errors.New("err"),
			isRunning: true,
		},
		{
			name: "isStopped",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				rows:    []*model.RedoRowChangedEvent{},
			},
			writerErr: cerror.ErrRedoWriterStopped,
			isRunning: false,
			wantErr:   cerror.ErrRedoWriterStopped,
		},
		{
			name: "context cancel",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				rows:    []*model.RedoRowChangedEvent{},
			},
			writerErr: nil,
			isRunning: true,
			wantErr:   context.Canceled,
		},
	}

	for _, tt := range tests {
		mockWriter := &mockFileWriter{}
		mockWriter.On("Write", mock.Anything).Return(1, tt.writerErr)
		mockWriter.On("IsRunning").Return(tt.isRunning)
		mockWriter.On("AdvanceTs", mock.Anything)
		writer := LogWriter{
			cfg: &LogWriterConfig{
				EmitMeta:      true,
				EmitRowEvents: true,
				EmitDDLEvents: true,
			},
			rowWriter: mockWriter,
			ddlWriter: mockWriter,
			meta:      &common.LogMeta{},
			metricTotalRowsCount: common.RedoTotalRowsCountGauge.
				WithLabelValues("default", ""),
		}
		if tt.name == "context cancel" {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tt.args.ctx = ctx
		}

		err := writer.WriteLog(tt.args.ctx, tt.args.tableID, tt.args.rows)
		if tt.wantErr != nil {
			log.Info("want error", zap.Error(tt.wantErr))
			log.Info("got error", zap.Error(err))
			require.Truef(t, errors.ErrorEqual(tt.wantErr, err), tt.name)
		} else {
			require.Nil(t, err, tt.name)
		}
	}
}

func TestLogWriterSendDDL(t *testing.T) {
	type arg struct {
		ctx     context.Context
		tableID int64
		ddl     *model.RedoDDLEvent
	}
	tests := []struct {
		name      string
		args      arg
		wantTs    uint64
		isRunning bool
		writerErr error
		wantErr   error
	}{
		{
			name: "happy",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ddl:     &model.RedoDDLEvent{DDL: &model.DDLEvent{CommitTs: 1}},
			},
			isRunning: true,
			writerErr: nil,
		},
		{
			name: "writer err",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ddl:     &model.RedoDDLEvent{DDL: &model.DDLEvent{CommitTs: 1}},
			},
			writerErr: errors.New("err"),
			wantErr:   errors.New("err"),
			isRunning: true,
		},
		{
			name: "ddl nil",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ddl:     nil,
			},
			writerErr: errors.New("err"),
			isRunning: true,
		},
		{
			name: "isStopped",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ddl:     &model.RedoDDLEvent{DDL: &model.DDLEvent{CommitTs: 1}},
			},
			writerErr: cerror.ErrRedoWriterStopped,
			isRunning: false,
			wantErr:   cerror.ErrRedoWriterStopped,
		},
		{
			name: "context cancel",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ddl:     &model.RedoDDLEvent{DDL: &model.DDLEvent{CommitTs: 1}},
			},
			writerErr: nil,
			isRunning: true,
			wantErr:   context.Canceled,
		},
	}

	for _, tt := range tests {
		mockWriter := &mockFileWriter{}
		mockWriter.On("Write", mock.Anything).Return(1, tt.writerErr)
		mockWriter.On("IsRunning").Return(tt.isRunning)
		mockWriter.On("AdvanceTs", mock.Anything)
		writer := LogWriter{
			cfg: &LogWriterConfig{
				EmitRowEvents: true,
				EmitDDLEvents: true,
			},
			rowWriter: mockWriter,
			ddlWriter: mockWriter,
			meta:      &common.LogMeta{},
		}

		if tt.name == "context cancel" {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tt.args.ctx = ctx
		}

		err := writer.SendDDL(tt.args.ctx, tt.args.ddl)
		if tt.wantErr != nil {
			require.True(t, errors.ErrorEqual(tt.wantErr, err), tt.name)
		} else {
			require.Nil(t, err, tt.name)
		}
	}
}

func TestLogWriterFlushLog(t *testing.T) {
	type arg struct {
		ctx     context.Context
		tableID int64
		ts      uint64
	}
	tests := []struct {
		name      string
		args      arg
		wantTs    uint64
		isRunning bool
		flushErr  error
		wantErr   error
	}{
		{
			name: "happy",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ts:      1,
			},
			isRunning: true,
			flushErr:  nil,
		},
		{
			name: "flush err",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ts:      1,
			},
			flushErr:  errors.New("err"),
			wantErr:   multierr.Append(errors.New("err"), errors.New("err")),
			isRunning: true,
		},
		{
			name: "isStopped",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ts:      1,
			},
			flushErr:  cerror.ErrRedoWriterStopped,
			isRunning: false,
			wantErr:   cerror.ErrRedoWriterStopped,
		},
		{
			name: "context cancel",
			args: arg{
				ctx:     context.Background(),
				tableID: 1,
				ts:      1,
			},
			flushErr:  nil,
			isRunning: true,
			wantErr:   context.Canceled,
		},
	}

	dir := t.TempDir()

	for _, tt := range tests {
		controller := gomock.NewController(t)
		mockStorage := mockstorage.NewMockExternalStorage(controller)
		if tt.isRunning && tt.name != "context cancel" {
			mockStorage.EXPECT().WriteFile(gomock.Any(),
				"cp_test-cf_meta.meta",
				gomock.Any()).Return(nil).Times(1)
		}
		mockWriter := &mockFileWriter{}
		mockWriter.On("Flush", mock.Anything).Return(tt.flushErr)
		mockWriter.On("IsRunning").Return(tt.isRunning)
		cfg := &LogWriterConfig{
			Dir:               dir,
			ChangeFeedID:      model.DefaultChangeFeedID("test-cf"),
			CaptureID:         "cp",
			MaxLogSize:        10,
			CreateTime:        time.Date(2000, 1, 1, 1, 1, 1, 1, &time.Location{}),
			FlushIntervalInMs: 5,
			S3Storage:         true,

			EmitMeta:      true,
			EmitRowEvents: true,
			EmitDDLEvents: true,
		}
		writer := LogWriter{
			cfg:       cfg,
			rowWriter: mockWriter,
			ddlWriter: mockWriter,
			meta:      &common.LogMeta{},
			storage:   mockStorage,
		}

		if tt.name == "context cancel" {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tt.args.ctx = ctx
		}
		err := writer.FlushLog(tt.args.ctx, 0, tt.args.ts)
		if tt.wantErr != nil {
			require.True(t, errors.ErrorEqual(tt.wantErr, err), err.Error()+tt.wantErr.Error())
		} else {
			require.Nil(t, err, tt.name)
		}
	}
}

// checkpoint or meta regress should be ignored correctly.
func TestLogWriterRegress(t *testing.T) {
	dir := t.TempDir()
	writer, err := NewLogWriter(context.Background(), &LogWriterConfig{
		Dir:          dir,
		ChangeFeedID: model.DefaultChangeFeedID("test-log-writer-regress"),
		CaptureID:    "cp",
		S3Storage:    false,

		EmitMeta:      true,
		EmitRowEvents: true,
		EmitDDLEvents: true,
	})
	require.Nil(t, err)
	require.Nil(t, writer.FlushLog(context.Background(), 2, 4))
	require.Nil(t, writer.FlushLog(context.Background(), 1, 3))
	require.Equal(t, uint64(2), writer.meta.CheckpointTs)
	require.Equal(t, uint64(4), writer.meta.ResolvedTs)
	_ = writer.Close()
}

func TestNewLogWriter(t *testing.T) {
	_, err := NewLogWriter(context.Background(), nil)
	require.NotNil(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := &LogWriterConfig{
		Dir:               "dirt",
		ChangeFeedID:      model.DefaultChangeFeedID("test-cf"),
		CaptureID:         "cp",
		MaxLogSize:        10,
		CreateTime:        time.Date(2000, 1, 1, 1, 1, 1, 1, &time.Location{}),
		FlushIntervalInMs: 5,

		EmitMeta:      true,
		EmitRowEvents: true,
		EmitDDLEvents: true,
	}
	uuidGen := uuid.NewConstGenerator("const-uuid")
	ll, err := NewLogWriter(ctx, cfg,
		WithUUIDGenerator(func() uuid.Generator { return uuidGen }),
	)
	require.Nil(t, err)

	cfg1 := &LogWriterConfig{
		Dir:               "dirt111",
		ChangeFeedID:      model.DefaultChangeFeedID("test-cf"),
		CaptureID:         "cp",
		MaxLogSize:        10,
		CreateTime:        time.Date(2000, 1, 1, 1, 1, 1, 1, &time.Location{}),
		FlushIntervalInMs: 5,
	}
	ll1, err := NewLogWriter(ctx, cfg1)
	require.Nil(t, err)
	require.NotSame(t, ll, ll1)

	dir := t.TempDir()
	cfg = &LogWriterConfig{
		Dir:               dir,
		ChangeFeedID:      model.DefaultChangeFeedID("test-cf"),
		CaptureID:         "cp",
		MaxLogSize:        10,
		CreateTime:        time.Date(2000, 1, 1, 1, 1, 1, 1, &time.Location{}),
		FlushIntervalInMs: 5,

		EmitMeta:      true,
		EmitRowEvents: true,
		EmitDDLEvents: true,
	}
	l, err := NewLogWriter(ctx, cfg)
	require.Nil(t, err)
	err = l.Close()
	require.Nil(t, err)
	path := l.filePath()
	f, err := os.Create(path)
	require.Nil(t, err)

	meta := &common.LogMeta{CheckpointTs: 11, ResolvedTs: 22}
	data, err := meta.MarshalMsg(nil)
	require.Nil(t, err)
	_, err = f.Write(data)
	require.Nil(t, err)

	l, err = NewLogWriter(ctx, cfg)
	require.Nil(t, err)
	err = l.Close()
	require.Nil(t, err)
	require.True(t, l.isStopped())
	require.Equal(t, cfg.Dir, l.cfg.Dir)
	require.Equal(t, meta.CheckpointTs, l.meta.CheckpointTs)
	require.Equal(t, meta.ResolvedTs, l.meta.ResolvedTs)

	origin := common.InitS3storage
	defer func() {
		common.InitS3storage = origin
	}()
	controller := gomock.NewController(t)
	mockStorage := mockstorage.NewMockExternalStorage(controller)
	// skip pre cleanup
	mockStorage.EXPECT().FileExists(gomock.Any(), gomock.Any()).Return(false, nil)
	common.InitS3storage = func(ctx context.Context, uri url.URL) (storage.ExternalStorage, error) {
		return mockStorage, nil
	}
	cfg3 := &LogWriterConfig{
		Dir:               dir,
		ChangeFeedID:      model.DefaultChangeFeedID("test-cf112232"),
		CaptureID:         "cp",
		MaxLogSize:        10,
		CreateTime:        time.Date(2000, 1, 1, 1, 1, 1, 1, &time.Location{}),
		FlushIntervalInMs: 5,
		S3Storage:         true,
	}
	l3, err := NewLogWriter(ctx, cfg3)
	require.Nil(t, err)
	err = l3.Close()
	require.Nil(t, err)
}

func TestDeleteAllLogs(t *testing.T) {
	fileName := "1"
	fileName1 := "11"

	type args struct {
		enableS3 bool
	}

	tests := []struct {
		name               string
		args               args
		closeErr           error
		getAllFilesInS3Err error
		deleteFileErr      error
		writeFileErr       error
		wantErr            string
	}{
		{
			name: "happy local",
			args: args{enableS3: false},
		},
		{
			name: "happy s3",
			args: args{enableS3: true},
		},
		{
			name:     "close err",
			args:     args{enableS3: true},
			closeErr: errors.New("xx"),
			wantErr:  ".*xx*.",
		},
		{
			name:               "getAllFilesInS3 err",
			args:               args{enableS3: true},
			getAllFilesInS3Err: errors.New("xx"),
			wantErr:            ".*xx*.",
		},
		{
			name:          "deleteFile normal err",
			args:          args{enableS3: true},
			deleteFileErr: errors.New("xx"),
			wantErr:       ".*ErrS3StorageAPI*.",
		},
		{
			name:          "deleteFile notExist err",
			args:          args{enableS3: true},
			deleteFileErr: awserr.New(s3.ErrCodeNoSuchKey, "no such key", nil),
		},
		{
			name:         "writerFile err",
			args:         args{enableS3: true},
			writeFileErr: errors.New("xx"),
			wantErr:      ".*xx*.",
		},
	}

	for _, tt := range tests {
		dir := t.TempDir()
		path := filepath.Join(dir, fileName)
		_, err := os.Create(path)
		require.Nil(t, err)
		path = filepath.Join(dir, fileName1)
		_, err = os.Create(path)
		require.Nil(t, err)

		origin := getAllFilesInS3
		getAllFilesInS3 = func(ctx context.Context, l *LogWriter) ([]string, error) {
			return []string{fileName, fileName1}, tt.getAllFilesInS3Err
		}
		controller := gomock.NewController(t)
		mockStorage := mockstorage.NewMockExternalStorage(controller)

		mockStorage.EXPECT().DeleteFile(gomock.Any(), gomock.Any()).Return(tt.deleteFileErr).MaxTimes(2)
		mockStorage.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.writeFileErr).MaxTimes(1)

		mockWriter := &mockFileWriter{}
		mockWriter.On("Close").Return(tt.closeErr)
		cfg := &LogWriterConfig{
			Dir:               dir,
			ChangeFeedID:      model.DefaultChangeFeedID("test-cf"),
			CaptureID:         "cp",
			MaxLogSize:        10,
			CreateTime:        time.Date(2000, 1, 1, 1, 1, 1, 1, &time.Location{}),
			FlushIntervalInMs: 5,
			S3Storage:         tt.args.enableS3,

			EmitMeta:      true,
			EmitRowEvents: true,
			EmitDDLEvents: true,
		}
		writer := LogWriter{
			rowWriter: mockWriter,
			ddlWriter: mockWriter,
			meta:      &common.LogMeta{},
			cfg:       cfg,
			storage:   mockStorage,
		}
		ret := writer.DeleteAllLogs(context.Background())
		if tt.wantErr != "" {
			require.Regexp(t, tt.wantErr, ret.Error(), tt.name)
		} else {
			require.Nil(t, ret, tt.name)
			if !tt.args.enableS3 {
				_, err := os.Stat(dir)
				require.True(t, os.IsNotExist(err), tt.name)
			}
		}
		getAllFilesInS3 = origin
	}
}

func TestPreCleanUpS3(t *testing.T) {
	testCases := []struct {
		name               string
		fileExistsErr      error
		fileExists         bool
		getAllFilesInS3Err error
		deleteFileErr      error
		wantErr            string
	}{
		{
			name:       "happy no marker",
			fileExists: false,
		},
		{
			name:          "fileExists err",
			fileExistsErr: errors.New("xx"),
			wantErr:       ".*xx*.",
		},
		{
			name:               "getAllFilesInS3 err",
			fileExists:         true,
			getAllFilesInS3Err: errors.New("xx"),
			wantErr:            ".*xx*.",
		},
		{
			name:          "deleteFile normal err",
			fileExists:    true,
			deleteFileErr: errors.New("xx"),
			wantErr:       ".*ErrS3StorageAPI*.",
		},
		{
			name:          "deleteFile notExist err",
			fileExists:    true,
			deleteFileErr: awserr.New(s3.ErrCodeNoSuchKey, "no such key", nil),
		},
	}

	for _, tc := range testCases {
		cfs := []model.ChangeFeedID{
			{
				Namespace: "abcd",
				ID:        "test-cf",
			},
			model.DefaultChangeFeedID("test-cf"),
		}
		for _, cf := range cfs {
			origin := getAllFilesInS3
			getAllFilesInS3 = func(ctx context.Context, l *LogWriter) ([]string, error) {
				if cf.Namespace == model.DefaultNamespace {
					return []string{"1", "11", "delete_test-cf"}, tc.getAllFilesInS3Err
				}
				return []string{"1", "11", "delete_abcd_test-cf"}, tc.getAllFilesInS3Err
			}
			controller := gomock.NewController(t)
			mockStorage := mockstorage.NewMockExternalStorage(controller)

			mockStorage.EXPECT().FileExists(gomock.Any(), gomock.Any()).
				Return(tc.fileExists, tc.fileExistsErr)
			mockStorage.EXPECT().DeleteFile(gomock.Any(), gomock.Any()).
				Return(tc.deleteFileErr).MaxTimes(3)

			cfg := &LogWriterConfig{
				Dir:          "dir",
				ChangeFeedID: cf,
				CaptureID:    "cp",
				MaxLogSize:   10,
				CreateTime: time.Date(2000, 1, 1, 1, 1, 1,
					1, &time.Location{}),
				FlushIntervalInMs: 5,
			}
			writer := LogWriter{
				cfg:     cfg,
				storage: mockStorage,
			}
			ret := writer.preCleanUpS3(context.Background())
			if tc.wantErr != "" {
				require.Regexp(t, tc.wantErr, ret.Error(), tc.name)
			} else {
				require.Nil(t, ret, tc.name)
			}
			getAllFilesInS3 = origin
		}
	}
}
