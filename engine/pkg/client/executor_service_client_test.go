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

package client

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pingcap/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pingcap/tiflow/engine/enginepb"
	pbMock "github.com/pingcap/tiflow/engine/enginepb/mock"
)

func TestDispatchTaskNormal(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := pbMock.NewMockExecutorServiceClient(ctrl)
	serviceCli := NewExecutorServiceClient(client)

	var (
		requestID           string
		preDispatchComplete atomic.Bool
		cbCalled            atomic.Bool
	)

	args := &DispatchTaskArgs{
		WorkerID:     "worker-1",
		MasterID:     "master-1",
		WorkerType:   1,
		WorkerConfig: []byte("testtest"),
	}

	client.EXPECT().PreDispatchTask(gomock.Any(), matchPreDispatchArgs(args)).
		Do(func(_ context.Context, arg1 *enginepb.PreDispatchTaskRequest, _ ...grpc.CallOption) {
			requestID = arg1.RequestId
			preDispatchComplete.Store(true)
		}).Return(&enginepb.PreDispatchTaskResponse{}, nil).Times(1)

	client.EXPECT().ConfirmDispatchTask(gomock.Any(), matchConfirmDispatch(&requestID, "worker-1")).
		Return(&enginepb.ConfirmDispatchTaskResponse{}, nil).Do(
		func(_ context.Context, _ *enginepb.ConfirmDispatchTaskRequest, _ ...grpc.CallOption) {
			require.True(t, preDispatchComplete.Load())
			require.True(t, cbCalled.Load())
		}).Times(1)

	err := serviceCli.DispatchTask(context.Background(), args, func() {
		require.True(t, preDispatchComplete.Load())
		require.False(t, cbCalled.Swap(true))
	}, func(error) {
		require.Fail(t, "not expected")
	})
	require.NoError(t, err)
}

func TestPreDispatchAborted(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := pbMock.NewMockExecutorServiceClient(ctrl)
	serviceCli := NewExecutorServiceClient(client)

	args := &DispatchTaskArgs{
		WorkerID:     "worker-1",
		MasterID:     "master-1",
		WorkerType:   1,
		WorkerConfig: []byte("testtest"),
	}

	var abortCalled atomic.Bool

	unknownRPCError := status.Error(codes.Unknown, "fake error")
	client.EXPECT().PreDispatchTask(gomock.Any(), matchPreDispatchArgs(args)).
		Return((*enginepb.PreDispatchTaskResponse)(nil), unknownRPCError).Times(1)

	err := serviceCli.DispatchTask(context.Background(), args, func() {
		t.Fatalf("unexpected callback")
	}, func(err error) {
		abortCalled.Swap(true)
	})
	require.Error(t, err)
	require.Regexp(t, "fake error", err)
	require.True(t, abortCalled.Load())
}

func TestConfirmDispatchErrorFailFast(t *testing.T) {
	t.Parallel()

	// Only those errors that indicates a server-side failure can
	// make DispatchTask fail fast. Otherwise, no error should be
	// reported and at least a timeout should be waited for.
	testCases := []struct {
		err        error
		isFailFast bool
	}{
		{
			err:        status.Error(codes.Aborted, "fake aborted error"),
			isFailFast: true,
		},
		{
			err:        status.Error(codes.NotFound, "fake not found error"),
			isFailFast: true,
		},
		{
			err:        errors.Trace(status.Error(codes.NotFound, "fake not found error")),
			isFailFast: true,
		},
		{
			err:        status.Error(codes.Canceled, "fake not found error"),
			isFailFast: false,
		},
		{
			err:        errors.New("some random error"),
			isFailFast: false,
		},
		{
			err:        context.Canceled,
			isFailFast: false,
		},
	}

	ctrl := gomock.NewController(t)
	client := pbMock.NewMockExecutorServiceClient(ctrl)
	serviceCli := NewExecutorServiceClient(client)

	for _, tc := range testCases {
		var (
			requestID           string
			preDispatchComplete atomic.Bool
			timerStarted        atomic.Bool
			aborted             atomic.Bool
		)

		args := &DispatchTaskArgs{
			WorkerID:     "worker-1",
			MasterID:     "master-1",
			WorkerType:   1,
			WorkerConfig: []byte("testtest"),
		}

		client.EXPECT().PreDispatchTask(gomock.Any(), matchPreDispatchArgs(args)).
			Do(func(_ context.Context, arg1 *enginepb.PreDispatchTaskRequest, _ ...grpc.CallOption) {
				requestID = arg1.RequestId
				preDispatchComplete.Store(true)
			}).Return(&enginepb.PreDispatchTaskResponse{}, nil).Times(1)

		client.EXPECT().ConfirmDispatchTask(gomock.Any(), matchConfirmDispatch(&requestID, "worker-1")).
			Return((*enginepb.ConfirmDispatchTaskResponse)(nil), tc.err).Do(
			func(_ context.Context, _ *enginepb.ConfirmDispatchTaskRequest, _ ...grpc.CallOption) {
				require.True(t, preDispatchComplete.Load())
				require.True(t, timerStarted.Load())
			}).Times(1)

		err := serviceCli.DispatchTask(context.Background(), args, func() {
			require.True(t, preDispatchComplete.Load())
			require.False(t, timerStarted.Swap(true))
		}, func(error) {
			require.False(t, aborted.Swap(true))
		})

		if tc.isFailFast {
			require.Error(t, err)
			require.True(t, aborted.Load())
		} else {
			require.NoError(t, err)
			require.False(t, aborted.Load())
		}
	}
}
