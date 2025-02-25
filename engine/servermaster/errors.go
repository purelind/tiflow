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

package servermaster

import "github.com/pingcap/tiflow/engine/pkg/rpcerror"

// ErrJobNotFound indicates that a given job cannot be found.
var ErrJobNotFound = rpcerror.Normalize[JobNotFoundError]()

// JobNotFoundError provides details of an ErrJobNotFound.
type JobNotFoundError struct {
	rpcerror.Error[rpcerror.NotRetryable, rpcerror.NotFound]

	JobID string
}

// ErrJobAlreadyExists indicates that a given job already exists.
var ErrJobAlreadyExists = rpcerror.Normalize[JobAlreadyExistsError]()

// JobAlreadyExistsError provides details of an ErrJobAlreadyExists.
type JobAlreadyExistsError struct {
	rpcerror.Error[rpcerror.NotRetryable, rpcerror.AlreadyExists]

	JobID string
}

// ErrJobNotStopped indicates that a given job is not stopped.
var ErrJobNotStopped = rpcerror.Normalize[JobNotStoppedError]()

// JobNotStoppedError provides details of an ErrJobNotStopped.
type JobNotStoppedError struct {
	rpcerror.Error[rpcerror.NotRetryable, rpcerror.FailedPrecondition]

	JobID string
}

// ErrJobNotRunning indicates that a given job is not running.
// It's usually caused when caller tries to cancel a job that is not running.
var ErrJobNotRunning = rpcerror.Normalize[JobNotRunningError]()

// JobNotRunningError provides details of an ErrJobNotRunning.
type JobNotRunningError struct {
	rpcerror.Error[rpcerror.NotRetryable, rpcerror.FailedPrecondition]

	JobID string
}
