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

package manager

import (
	"context"
	"sync"

	"github.com/pingcap/log"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/pingcap/tiflow/engine/enginepb"
	"github.com/pingcap/tiflow/engine/model"
	"github.com/pingcap/tiflow/engine/pkg/externalresource/internal"
	resModel "github.com/pingcap/tiflow/engine/pkg/externalresource/resourcemeta/model"
	pkgOrm "github.com/pingcap/tiflow/engine/pkg/orm"
	"github.com/pingcap/tiflow/engine/pkg/rpcutil"
	"github.com/pingcap/tiflow/engine/pkg/tenant"
	"github.com/pingcap/tiflow/pkg/errors"
)

// Service implements pb.ResourceManagerServer
type Service struct {
	metaclient pkgOrm.Client

	executors ExecutorInfoProvider

	wg       sync.WaitGroup
	cancelCh chan struct{}

	preRPCHook rpcutil.PreRPCHook
}

// NewService creates a new externalresource manage service
func NewService(
	metaclient pkgOrm.Client,
	executorInfoProvider ExecutorInfoProvider,
	preRPCHook rpcutil.PreRPCHook,
) *Service {
	return &Service{
		metaclient: metaclient,
		executors:  executorInfoProvider,
		preRPCHook: preRPCHook,
	}
}

// QueryResource implements ResourceManagerClient.QueryResource
func (s *Service) QueryResource(
	ctx context.Context,
	request *pb.QueryResourceRequest,
) (*pb.QueryResourceResponse, error) {
	var resp2 *pb.QueryResourceResponse
	shouldRet, err := s.preRPCHook.PreRPC(ctx, request, &resp2)
	if shouldRet {
		return resp2, err
	}

	jobID := request.GetResourceKey().GetJobId()
	resourceID := request.GetResourceKey().GetResourceId()

	if err := checkArguments(resourceID, jobID); err != nil {
		return nil, err
	}

	record, err := s.metaclient.GetResourceByID(ctx, pkgOrm.ResourceKey{JobID: jobID, ID: resourceID})
	if err != nil {
		if pkgOrm.IsNotFoundError(err) {
			return nil, internal.ErrResourceNotFound.GenWithStack(&internal.ResourceNotFoundError{
				ResourceID: resourceID,
				Details:    err.Error(),
			})
		}
		return nil, internal.ErrResourceMetastoreError.GenWithStack(&internal.ResourceMetastoreError{
			ResourceID: resourceID,
			Details:    err.Error(),
		})
	}

	if record.Deleted {
		// This logic is currently not used.
		return nil, status.Error(codes.NotFound, "resource marked as deleted")
	}
	return record.ToQueryResourceResponse(), nil
}

// CreateResource implements ResourceManagerClient.CreateResource
func (s *Service) CreateResource(
	ctx context.Context,
	request *pb.CreateResourceRequest,
) (*pb.CreateResourceResponse, error) {
	var resp2 *pb.CreateResourceResponse
	shouldRet, err := s.preRPCHook.PreRPC(ctx, request, &resp2)
	if shouldRet {
		return resp2, err
	}

	if err := checkArguments(request.GetResourceId(), request.GetJobId()); err != nil {
		return nil, err
	}

	resourceRecord := &resModel.ResourceMeta{
		ProjectID: tenant.NewProjectInfo(request.GetProjectInfo().TenantId, request.GetProjectInfo().ProjectId).UniqueID(),
		ID:        request.GetResourceId(),
		Job:       request.GetJobId(),
		Worker:    request.GetCreatorWorkerId(),
		Executor:  resModel.ExecutorID(request.GetCreatorExecutor()),
		Deleted:   false,
	}

	err = s.metaclient.CreateResource(ctx, resourceRecord)
	if errors.ErrDuplicateResourceID.Equal(err) {
		return nil, internal.ErrResourceAlreadyExists.GenWithStack(
			&internal.ResourceAlreadyExistsError{
				ResourceID: request.GetResourceId(),
				Details:    err.Error(),
			})
	}
	if err != nil {
		return nil, internal.ErrResourceMetastoreError.GenWithStack(
			&internal.ResourceMetastoreError{
				ResourceID: request.GetResourceId(),
				Details:    err.Error(),
			})
	}

	return &pb.CreateResourceResponse{}, nil
}

// RemoveResource implements ResourceManagerClient.RemoveResource
func (s *Service) RemoveResource(
	ctx context.Context,
	request *pb.RemoveResourceRequest,
) (*pb.RemoveResourceResponse, error) {
	var resp2 *pb.RemoveResourceResponse
	shouldRet, err := s.preRPCHook.PreRPC(ctx, request, &resp2)
	if shouldRet {
		return resp2, err
	}

	jobID := request.GetResourceKey().GetJobId()
	resourceID := request.GetResourceKey().GetResourceId()
	if err := checkArguments(resourceID, jobID); err != nil {
		return nil, err
	}

	res, err := s.metaclient.DeleteResource(ctx, pkgOrm.ResourceKey{JobID: jobID, ID: resourceID})
	if err != nil {
		return nil, internal.ErrResourceMetastoreError.GenWithStack(
			&internal.ResourceMetastoreError{
				ResourceID: resourceID,
				Details:    err.Error(),
			})
	}

	if res.RowsAffected() == 0 {
		return nil, internal.ErrResourceNotFound.GenWithStack(
			&internal.ResourceNotFoundError{
				ResourceID: resourceID,
				Details:    err.Error(),
			})
	}
	if res.RowsAffected() > 1 {
		log.Panic("unexpected RowsAffected",
			zap.String("job-id", jobID),
			zap.String("resource-id", resourceID))
	}

	return &pb.RemoveResourceResponse{}, nil
}

// GetPlacementConstraint is called by the Scheduler to determine whether
// a resource the worker relies on requires the worker running on a specific
// executor.
// Returns:
// (1) A local resource is required and the resource exists: (executorID, true, nil)
// (2) A local resource is required but the resource is not found: ("", false, ErrResourceDoesNotExist)
// (3) No placement constraint is needed: ("", false, nil)
// (4) Other errors: ("", false, err)
func (s *Service) GetPlacementConstraint(
	ctx context.Context,
	resourceKey resModel.ResourceKey,
) (resModel.ExecutorID, bool, error) {
	logger := log.With(
		zap.String("job-id", resourceKey.JobID),
		zap.String("resource-id", resourceKey.ID))

	rType, _, err := resModel.ParseResourcePath(resourceKey.ID)
	if err != nil {
		return "", false, err
	}

	if rType != resModel.ResourceTypeLocalFile {
		logger.Info("Resource does not need a constraint",
			zap.String("resource-id", resourceKey.ID), zap.String("type", string(rType)))
		return "", false, nil
	}

	record, err := s.metaclient.GetResourceByID(ctx, pkgOrm.ResourceKey{JobID: resourceKey.JobID, ID: resourceKey.ID})
	if err != nil {
		if pkgOrm.IsNotFoundError(err) {
			return "", false, errors.ErrResourceDoesNotExist.GenWithStackByArgs(resourceKey.ID)
		}
		return "", false, err
	}

	if record.Deleted {
		logger.Info("Resource meta is marked as deleted", zap.Any("record", record))
		return "", false, errors.ErrResourceDoesNotExist.GenWithStackByArgs(resourceKey.ID)
	}
	return record.Executor, true, nil
}

func checkArguments(resourceID resModel.ResourceID, jobID model.JobID) error {
	if resourceID == "" {
		return internal.ErrInvalidArgument.GenWithStack(&internal.InvalidArgumentError{
			JobID:      jobID,
			Annotation: "resource-id cannot be empty",
		})
	}

	if jobID == "" {
		return internal.ErrInvalidArgument.GenWithStack(&internal.InvalidArgumentError{
			ResourceID: resourceID,
			Annotation: "job-id cannot be empty",
		})
	}
	return nil
}
