// Package openapi provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen version v1.9.0 DO NOT EDIT.
package openapi

import (
	"encoding/json"
	"fmt"
)

// Defines values for TaskOnDuplicate.
const (
	TaskOnDuplicateError TaskOnDuplicate = "error"

	TaskOnDuplicateIgnore TaskOnDuplicate = "ignore"

	TaskOnDuplicateReplace TaskOnDuplicate = "replace"
)

// Defines values for TaskShardMode.
const (
	TaskShardModeOptimistic TaskShardMode = "optimistic"

	TaskShardModePessimistic TaskShardMode = "pessimistic"
)

// Defines values for TaskTaskMode.
const (
	TaskTaskModeAll TaskTaskMode = "all"

	TaskTaskModeDump TaskTaskMode = "dump"

	TaskTaskModeFull TaskTaskMode = "full"

	TaskTaskModeIncremental TaskTaskMode = "incremental"
)

// Defines values for TaskFullMigrateConfAnalyze.
const (
	TaskFullMigrateConfAnalyzeOff TaskFullMigrateConfAnalyze = "off"

	TaskFullMigrateConfAnalyzeOptional TaskFullMigrateConfAnalyze = "optional"

	TaskFullMigrateConfAnalyzeRequired TaskFullMigrateConfAnalyze = "required"
)

// Defines values for TaskFullMigrateConfChecksum.
const (
	TaskFullMigrateConfChecksumOff TaskFullMigrateConfChecksum = "off"

	TaskFullMigrateConfChecksumOptional TaskFullMigrateConfChecksum = "optional"

	TaskFullMigrateConfChecksumRequired TaskFullMigrateConfChecksum = "required"
)

// Defines values for TaskFullMigrateConfImportMode.
const (
	TaskFullMigrateConfImportModeLogical TaskFullMigrateConfImportMode = "logical"

	TaskFullMigrateConfImportModePhysical TaskFullMigrateConfImportMode = "physical"
)

// Defines values for TaskFullMigrateConfOnDuplicateLogical.
const (
	TaskFullMigrateConfOnDuplicateLogicalError TaskFullMigrateConfOnDuplicateLogical = "error"

	TaskFullMigrateConfOnDuplicateLogicalIgnore TaskFullMigrateConfOnDuplicateLogical = "ignore"

	TaskFullMigrateConfOnDuplicateLogicalReplace TaskFullMigrateConfOnDuplicateLogical = "replace"
)

// Defines values for TaskFullMigrateConfOnDuplicatePhysical.
const (
	TaskFullMigrateConfOnDuplicatePhysicalManual TaskFullMigrateConfOnDuplicatePhysical = "manual"

	TaskFullMigrateConfOnDuplicatePhysicalNone TaskFullMigrateConfOnDuplicatePhysical = "none"
)

// Defines values for TaskStage.
const (
	TaskStageFinished TaskStage = "Finished"

	TaskStagePaused TaskStage = "Paused"

	TaskStageRunning TaskStage = "Running"

	TaskStageStopped TaskStage = "Stopped"
)

// AlertManagerTopology defines model for AlertManagerTopology.
type AlertManagerTopology struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// ClusterMaster defines model for ClusterMaster.
type ClusterMaster struct {
	// address of the current master node
	Addr string `json:"addr"`

	// online status of this master
	Alive bool `json:"alive"`

	// is this master the leader
	Leader bool   `json:"leader"`
	Name   string `json:"name"`
}

// ClusterTopology defines model for ClusterTopology.
type ClusterTopology struct {
	AlertManagerTopology *AlertManagerTopology `json:"alert_manager_topology,omitempty"`
	GrafanaTopology      *GrafanaTopology      `json:"grafana_topology,omitempty"`
	MasterTopologyList   *[]MasterTopology     `json:"master_topology_list,omitempty"`
	PrometheusTopology   *PrometheusTopology   `json:"prometheus_topology,omitempty"`
	WorkerTopologyList   *[]WorkerTopology     `json:"worker_topology_list,omitempty"`
}

// ClusterWorker defines model for ClusterWorker.
type ClusterWorker struct {
	// address of the current master node
	Addr string `json:"addr"`

	// source name bound to this worker node
	BoundSourceName string `json:"bound_source_name"`

	// bound stage of this worker node
	BoundStage string `json:"bound_stage"`
	Name       string `json:"name"`
}

// ConverterTaskRequest defines model for ConverterTaskRequest.
type ConverterTaskRequest struct {
	// task
	Task *Task `json:"task,omitempty"`

	// config file in yaml format https://docs.pingcap.com/zh/tidb/stable/task-configuration-file-full.
	TaskConfigFile *string `json:"task_config_file,omitempty"`
}

// ConverterTaskResponse defines model for ConverterTaskResponse.
type ConverterTaskResponse struct {
	// task
	Task Task `json:"task"`

	// config file in yaml format https://docs.pingcap.com/zh/tidb/stable/task-configuration-file-full.
	TaskConfigFile string `json:"task_config_file"`
}

// CreateSourceRequest defines model for CreateSourceRequest.
type CreateSourceRequest struct {
	// source
	Source     Source  `json:"source"`
	WorkerName *string `json:"worker_name,omitempty"`
}

// CreateTaskRequest defines model for CreateTaskRequest.
type CreateTaskRequest struct {
	// task
	Task Task `json:"task"`
}

// action to stop a relay request
type DisableRelayRequest struct {
	// worker name list
	WorkerNameList *WorkerNameList `json:"worker_name_list,omitempty"`
}

// status of dump unit
type DumpStatus struct {
	Bps               int64   `json:"bps"`
	CompletedTables   float64 `json:"completed_tables"`
	EstimateTotalRows float64 `json:"estimate_total_rows"`
	FinishedBytes     float64 `json:"finished_bytes"`
	FinishedRows      float64 `json:"finished_rows"`
	Progress          string  `json:"progress"`
	TotalTables       int64   `json:"total_tables"`
}

// action to start a relay request
type EnableRelayRequest struct {
	// starting GTID of the upstream binlog
	RelayBinlogGtid *string `json:"relay_binlog_gtid"`

	// starting filename of the upstream binlog
	RelayBinlogName *string `json:"relay_binlog_name"`

	// the directory where the relay log is stored
	RelayDir *string `json:"relay_dir"`

	// worker name list
	WorkerNameList *WorkerNameList `json:"worker_name_list,omitempty"`
}

// operation error
type ErrorWithMessage struct {
	// error code
	ErrorCode int `json:"error_code"`

	// error message
	ErrorMsg string `json:"error_msg"`
}

// GetClusterInfoResponse defines model for GetClusterInfoResponse.
type GetClusterInfoResponse struct {
	// cluster id
	ClusterId uint64           `json:"cluster_id"`
	Topology  *ClusterTopology `json:"topology,omitempty"`
}

// GetClusterMasterListResponse defines model for GetClusterMasterListResponse.
type GetClusterMasterListResponse struct {
	Data  []ClusterMaster `json:"data"`
	Total int             `json:"total"`
}

// GetClusterWorkerListResponse defines model for GetClusterWorkerListResponse.
type GetClusterWorkerListResponse struct {
	Data  []ClusterWorker `json:"data"`
	Total int             `json:"total"`
}

// GetSourceListResponse defines model for GetSourceListResponse.
type GetSourceListResponse struct {
	Data  []Source `json:"data"`
	Total int      `json:"total"`
}

// GetSourceStatusResponse defines model for GetSourceStatusResponse.
type GetSourceStatusResponse struct {
	Data  []SourceStatus `json:"data"`
	Total int            `json:"total"`
}

// GetTaskListResponse defines model for GetTaskListResponse.
type GetTaskListResponse struct {
	Data  []Task `json:"data"`
	Total int    `json:"total"`
}

// GetTaskMigrateTargetsResponse defines model for GetTaskMigrateTargetsResponse.
type GetTaskMigrateTargetsResponse struct {
	Data  []TaskMigrateTarget `json:"data"`
	Total int                 `json:"total"`
}

// GetTaskStatusResponse defines model for GetTaskStatusResponse.
type GetTaskStatusResponse struct {
	Data  []SubTaskStatus `json:"data"`
	Total int             `json:"total"`
}

// GetTaskTableStructureResponse defines model for GetTaskTableStructureResponse.
type GetTaskTableStructureResponse struct {
	SchemaCreateSql *string `json:"schema_create_sql,omitempty"`
	SchemaName      *string `json:"schema_name,omitempty"`
	TableName       string  `json:"table_name"`
}

// GrafanaTopology defines model for GrafanaTopology.
type GrafanaTopology struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// status of load unit
type LoadStatus struct {
	Bps            int64  `json:"bps"`
	FinishedBytes  int64  `json:"finished_bytes"`
	MetaBinlog     string `json:"meta_binlog"`
	MetaBinlogGtid string `json:"meta_binlog_gtid"`
	Progress       string `json:"progress"`
	TotalBytes     int64  `json:"total_bytes"`
}

// MasterTopology defines model for MasterTopology.
type MasterTopology struct {
	Host string `json:"host"`
	Name string `json:"name"`
	Port int    `json:"port"`
}

// OperateTaskResponse defines model for OperateTaskResponse.
type OperateTaskResponse struct {
	// pre-check result
	CheckResult string `json:"check_result"`

	// task
	Task Task `json:"task"`
}

// action to operate table request
type OperateTaskTableStructureRequest struct {
	// Writes the schema to the checkpoint so that DM can load it after restarting the task
	Flush *bool `json:"flush,omitempty"`

	// sql you want to operate
	SqlContent string `json:"sql_content"`

	// Updates the optimistic sharding metadata with this schema only used when an error occurs in the optimistic sharding DDL mode
	Sync *bool `json:"sync,omitempty"`
}

// PrometheusTopology defines model for PrometheusTopology.
type PrometheusTopology struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// relay log cleanup policy configuration
type Purge struct {
	// expiration time of relay log
	Expires *int64 `json:"expires"`

	// The interval to periodically check if the relay log is expired, default value: 3600, in seconds
	Interval *int64 `json:"interval"`

	// Minimum free disk space, in GB
	RemainSpace *int64 `json:"remain_space"`
}

// action to stop a relay request
type PurgeRelayRequest struct {
	// starting filename of the upstream binlog
	RelayBinlogName string `json:"relay_binlog_name"`

	// specify relay sub directory for relay_binlog_name. If not specified, the latest one will be used. Sample format: 2ae76434-f79f-11e8-bde2-0242ac130008.000001
	RelayDir *string `json:"relay_dir"`
}

// the config of relay
type RelayConfig struct {
	EnableRelay *bool `json:"enable_relay,omitempty"`

	// starting GTID of the upstream binlog
	RelayBinlogGtid *string `json:"relay_binlog_gtid"`

	// starting filename of the upstream binlog
	RelayBinlogName *string `json:"relay_binlog_name"`

	// the directory where the relay log is stored
	RelayDir *string `json:"relay_dir"`
}

// status of relay log
type RelayStatus struct {
	// upstream binlog file information
	MasterBinlog string `json:"master_binlog"`

	// GTID of the upstream
	MasterBinlogGtid string `json:"master_binlog_gtid"`

	// relay current GTID
	RelayBinlogGtid string `json:"relay_binlog_gtid"`

	// whether to catch up with upstream progress
	RelayCatchUpMaster bool `json:"relay_catch_up_master"`

	// the directory where the relay log is stored
	RelayDir string `json:"relay_dir"`

	// current status
	Stage string `json:"stage"`
}

// schema name list
type SchemaNameList []string

// data source ssl configuration, the field will be hidden when getting the data source configuration from the interface
type Security struct {
	// Common Name of SSL certificates
	CertAllowedCn *[]string `json:"cert_allowed_cn,omitempty"`

	// certificate file content
	SslCaContent string `json:"ssl_ca_content"`

	// File content of PEM format/X509 format certificates
	SslCertContent string `json:"ssl_cert_content"`

	// Content of the private key file in X509 format
	SslKeyContent string `json:"ssl_key_content"`
}

// ShardingGroup defines model for ShardingGroup.
type ShardingGroup struct {
	DdlList       []string `json:"ddl_list"`
	FirstLocation string   `json:"first_location"`
	Synced        []string `json:"synced"`
	Target        string   `json:"target"`
	Unsynced      []string `json:"unsynced"`
}

// source
type Source struct {
	// whether this source is enabled
	Enable bool `json:"enable"`

	// whether to use GTID to pull binlogs from upstream
	EnableGtid bool `json:"enable_gtid"`

	// flavor of this source
	Flavor *string `json:"flavor,omitempty"`

	// source address
	Host string `json:"host"`

	// source password
	Password *string `json:"password"`

	// source port
	Port int `json:"port"`

	// relay log cleanup policy configuration
	Purge *Purge `json:"purge,omitempty"`

	// the config of relay
	RelayConfig *RelayConfig `json:"relay_config,omitempty"`

	// data source ssl configuration, the field will be hidden when getting the data source configuration from the interface
	Security *Security `json:"security"`

	// source name
	SourceName string          `json:"source_name"`
	StatusList *[]SourceStatus `json:"status_list,omitempty"`

	// task name list
	TaskNameList *TaskNameList `json:"task_name_list,omitempty"`

	// source username
	User string `json:"user"`
}

// source name list
type SourceNameList []string

// source status
type SourceStatus struct {
	// error message when something wrong
	ErrorMsg *string `json:"error_msg,omitempty"`

	// status of relay log
	RelayStatus *RelayStatus `json:"relay_status,omitempty"`

	// source name
	SourceName string `json:"source_name"`

	// The worker currently bound to the source
	WorkerName string `json:"worker_name"`
}

// StartTaskRequest defines model for StartTaskRequest.
type StartTaskRequest struct {
	// whether to remove meta database in downstream database
	RemoveMeta *bool `json:"remove_meta,omitempty"`

	// time duration of safe mode
	SafeModeTimeDuration *string `json:"safe_mode_time_duration,omitempty"`

	// source name list
	SourceNameList *SourceNameList `json:"source_name_list,omitempty"`

	// task start time
	StartTime *string `json:"start_time,omitempty"`
}

// StopTaskRequest defines model for StopTaskRequest.
type StopTaskRequest struct {
	// source name list
	SourceNameList *SourceNameList `json:"source_name_list,omitempty"`

	// time duration waiting task stop
	TimeoutDuration *string `json:"timeout_duration,omitempty"`
}

// SubTaskStatus defines model for SubTaskStatus.
type SubTaskStatus struct {
	// status of dump unit
	DumpStatus *DumpStatus `json:"dump_status,omitempty"`

	// error message when something wrong
	ErrorMsg *string `json:"error_msg,omitempty"`

	// status of load unit
	LoadStatus *LoadStatus `json:"load_status,omitempty"`

	// task name
	Name string `json:"name"`

	// source name
	SourceName string    `json:"source_name"`
	Stage      TaskStage `json:"stage"`

	// status of sync unit
	SyncStatus *SyncStatus `json:"sync_status,omitempty"`

	// task unit type
	Unit                string  `json:"unit"`
	UnresolvedDdlLockId *string `json:"unresolved_ddl_lock_id,omitempty"`

	// worker name
	WorkerName string `json:"worker_name"`
}

// status of sync unit
type SyncStatus struct {
	BinlogType string `json:"binlog_type"`

	// sharding DDL which current is blocking
	BlockingDdls        []string `json:"blocking_ddls"`
	DumpIoTotalBytes    uint64   `json:"dump_io_total_bytes"`
	IoTotalBytes        uint64   `json:"io_total_bytes"`
	MasterBinlog        string   `json:"master_binlog"`
	MasterBinlogGtid    string   `json:"master_binlog_gtid"`
	RecentTps           int64    `json:"recent_tps"`
	SecondsBehindMaster int64    `json:"seconds_behind_master"`
	Synced              bool     `json:"synced"`
	SyncerBinlog        string   `json:"syncer_binlog"`
	SyncerBinlogGtid    string   `json:"syncer_binlog_gtid"`
	TotalEvents         int64    `json:"total_events"`
	TotalTps            int64    `json:"total_tps"`

	// sharding groups which current are un-resolved
	UnresolvedGroups []ShardingGroup `json:"unresolved_groups"`
}

// schema name list
type TableNameList []string

// task
type Task struct {
	BinlogFilterRule *Task_BinlogFilterRule `json:"binlog_filter_rule,omitempty"`

	// whether to enable support for the online ddl plugin
	EnhanceOnlineSchemaChange bool `json:"enhance_online_schema_change"`

	// ignore precheck items
	IgnoreCheckingItems *[]string `json:"ignore_checking_items,omitempty"`

	// downstream database for storing meta information
	MetaSchema *string `json:"meta_schema,omitempty"`

	// task name
	Name string `json:"name"`

	// how to handle conflicted data
	OnDuplicate TaskOnDuplicate `json:"on_duplicate"`

	// the way to coordinate DDL
	ShardMode *TaskShardMode `json:"shard_mode,omitempty"`

	// source-related configuration
	SourceConfig TaskSourceConfig `json:"source_config"`
	StatusList   *[]SubTaskStatus `json:"status_list,omitempty"`

	// whether to enable strict optimistic shard mode
	StrictOptimisticShardMode *bool `json:"strict_optimistic_shard_mode,omitempty"`

	// table migrate rule
	TableMigrateRule []TaskTableMigrateRule `json:"table_migrate_rule"`

	// downstream database configuration
	TargetConfig TaskTargetDataBase `json:"target_config"`

	// migrate mode
	TaskMode TaskTaskMode `json:"task_mode"`
}

// Task_BinlogFilterRule defines model for Task.BinlogFilterRule.
type Task_BinlogFilterRule struct {
	AdditionalProperties map[string]TaskBinLogFilterRule `json:"-"`
}

// how to handle conflicted data
type TaskOnDuplicate string

// the way to coordinate DDL
type TaskShardMode string

// migrate mode
type TaskTaskMode string

// Filtering rules at binlog level
type TaskBinLogFilterRule struct {
	// event type
	IgnoreEvent *[]string `json:"ignore_event,omitempty"`

	// sql pattern to filter
	IgnoreSql *[]string `json:"ignore_sql,omitempty"`
}

// configuration of full migrate tasks
type TaskFullMigrateConf struct {
	// to control checksum of physical import
	Analyze *TaskFullMigrateConfAnalyze `json:"analyze,omitempty"`

	// to control checksum of physical import
	Checksum *TaskFullMigrateConfChecksum `json:"checksum,omitempty"`

	// to control compress kv pairs of physical import
	CompressKvPairs *string `json:"compress-kv-pairs,omitempty"`

	// to control the way in which data is exported for consistency assurance
	Consistency *string `json:"consistency,omitempty"`

	// storage dir name
	DataDir *string `json:"data_dir,omitempty"`

	// disk quota for physical import
	DiskQuota *string `json:"disk_quota,omitempty"`

	// full export of concurrent
	ExportThreads *int `json:"export_threads,omitempty"`

	// to control import mode of full import
	ImportMode *TaskFullMigrateConfImportMode `json:"import_mode,omitempty"`

	// full import of concurrent
	ImportThreads *int `json:"import_threads,omitempty"`

	// to control the duplication resolution when meet duplicate rows for logical import
	OnDuplicateLogical *TaskFullMigrateConfOnDuplicateLogical `json:"on_duplicate_logical,omitempty"`

	// to control the duplication resolution when meet duplicate rows for physical import
	OnDuplicatePhysical *TaskFullMigrateConfOnDuplicatePhysical `json:"on_duplicate_physical,omitempty"`

	// address of pd
	PdAddr *string `json:"pd_addr,omitempty"`

	// to control range concurrency of physical import
	RangeConcurrency *int `json:"range_concurrency,omitempty"`

	// sorting dir name for physical import
	SortingDir *string `json:"sorting_dir,omitempty"`
}

// to control checksum of physical import
type TaskFullMigrateConfAnalyze string

// to control checksum of physical import
type TaskFullMigrateConfChecksum string

// to control import mode of full import
type TaskFullMigrateConfImportMode string

// to control the duplication resolution when meet duplicate rows for logical import
type TaskFullMigrateConfOnDuplicateLogical string

// to control the duplication resolution when meet duplicate rows for physical import
type TaskFullMigrateConfOnDuplicatePhysical string

// configuration of incremental tasks
type TaskIncrMigrateConf struct {
	// incremental synchronization of batch execution sql quantities
	ReplBatch *int `json:"repl_batch,omitempty"`

	// incremental task of concurrent
	ReplThreads *int `json:"repl_threads,omitempty"`
}

// task migrate targets
type TaskMigrateTarget struct {
	SourceSchema string `json:"source_schema"`
	SourceTable  string `json:"source_table"`
	TargetSchema string `json:"target_schema"`
	TargetTable  string `json:"target_table"`
}

// task name list
type TaskNameList []string

// TaskSourceConf defines model for TaskSourceConf.
type TaskSourceConf struct {
	BinlogGtid *string `json:"binlog_gtid,omitempty"`
	BinlogName *string `json:"binlog_name,omitempty"`
	BinlogPos  *int    `json:"binlog_pos,omitempty"`

	// source name
	SourceName string `json:"source_name"`
}

// source-related configuration
type TaskSourceConfig struct {
	// configuration of full migrate tasks
	FullMigrateConf *TaskFullMigrateConf `json:"full_migrate_conf,omitempty"`

	// configuration of incremental tasks
	IncrMigrateConf *TaskIncrMigrateConf `json:"incr_migrate_conf,omitempty"`

	// source configuration
	SourceConf []TaskSourceConf `json:"source_conf"`
}

// TaskStage defines model for TaskStage.
type TaskStage string

// upstream table to downstream migrate rules
type TaskTableMigrateRule struct {
	// filter rule name
	BinlogFilterRule *[]string `json:"binlog_filter_rule,omitempty"`

	// source-related configuration
	Source struct {
		// schema name, wildcard support
		Schema string `json:"schema"`

		// source name
		SourceName string `json:"source_name"`

		// table name, wildcard support
		Table string `json:"table"`
	} `json:"source"`

	// downstream-related configuration
	Target *struct {
		// schema name, does not support wildcards
		Schema *string `json:"schema,omitempty"`

		// table name, does not support wildcards
		Table *string `json:"table,omitempty"`
	} `json:"target,omitempty"`
}

// downstream database configuration
type TaskTargetDataBase struct {
	// source address
	Host string `json:"host"`

	// source password
	Password string `json:"password"`

	// source port
	Port int `json:"port"`

	// data source ssl configuration, the field will be hidden when getting the data source configuration from the interface
	Security *Security `json:"security"`

	// source username
	User string `json:"user"`
}

// TaskTemplateRequest defines model for TaskTemplateRequest.
type TaskTemplateRequest struct {
	// whether to overwrite task template template
	Overwrite bool `json:"overwrite"`
}

// TaskTemplateResponse defines model for TaskTemplateResponse.
type TaskTemplateResponse struct {
	FailedTaskList []struct {
		ErrorMsg string `json:"error_msg"`
		TaskName string `json:"task_name"`
	} `json:"failed_task_list"`
	SuccessTaskList []string `json:"success_task_list"`
}

// UpdateSourceRequest defines model for UpdateSourceRequest.
type UpdateSourceRequest struct {
	// source
	Source Source `json:"source"`
}

// UpdateTaskRequest defines model for UpdateTaskRequest.
type UpdateTaskRequest struct {
	// task
	Task Task `json:"task"`
}

// worker name list
type WorkerNameList []string

// requests related to workers
type WorkerNameRequest struct {
	// worker name
	WorkerName string `json:"worker_name"`
}

// WorkerTopology defines model for WorkerTopology.
type WorkerTopology struct {
	Host string `json:"host"`
	Name string `json:"name"`
	Port int    `json:"port"`
}

// DMAPIUpdateClusterInfoJSONBody defines parameters for DMAPIUpdateClusterInfo.
type DMAPIUpdateClusterInfoJSONBody ClusterTopology

// DMAPIGetSourceListParams defines parameters for DMAPIGetSourceList.
type DMAPIGetSourceListParams struct {
	// list source with status
	WithStatus *bool `json:"with_status,omitempty"`

	// only return the enable-relay source
	EnableRelay *bool `json:"enable_relay,omitempty"`
}

// DMAPICreateSourceJSONBody defines parameters for DMAPICreateSource.
type DMAPICreateSourceJSONBody CreateSourceRequest

// DMAPIDeleteSourceParams defines parameters for DMAPIDeleteSource.
type DMAPIDeleteSourceParams struct {
	// force stop source also stop the related tasks
	Force *bool `json:"force,omitempty"`
}

// DMAPIGetSourceParams defines parameters for DMAPIGetSource.
type DMAPIGetSourceParams struct {
	// list source with status
	WithStatus *bool `json:"with_status,omitempty"`
}

// DMAPIUpdateSourceJSONBody defines parameters for DMAPIUpdateSource.
type DMAPIUpdateSourceJSONBody UpdateSourceRequest

// DMAPIDisableRelayJSONBody defines parameters for DMAPIDisableRelay.
type DMAPIDisableRelayJSONBody DisableRelayRequest

// DMAPIEnableRelayJSONBody defines parameters for DMAPIEnableRelay.
type DMAPIEnableRelayJSONBody EnableRelayRequest

// DMAPIPurgeRelayJSONBody defines parameters for DMAPIPurgeRelay.
type DMAPIPurgeRelayJSONBody PurgeRelayRequest

// DMAPITransferSourceJSONBody defines parameters for DMAPITransferSource.
type DMAPITransferSourceJSONBody WorkerNameRequest

// DMAPIGetTaskListParams defines parameters for DMAPIGetTaskList.
type DMAPIGetTaskListParams struct {
	// get task with status
	WithStatus *bool `json:"with_status,omitempty"`

	// filter by task stage
	Stage *TaskStage `json:"stage,omitempty"`

	// filter by source name
	SourceNameList *SourceNameList `json:"source_name_list,omitempty"`
}

// DMAPICreateTaskJSONBody defines parameters for DMAPICreateTask.
type DMAPICreateTaskJSONBody CreateTaskRequest

// DMAPIConvertTaskJSONBody defines parameters for DMAPIConvertTask.
type DMAPIConvertTaskJSONBody ConverterTaskRequest

// DMAPICreateTaskTemplateJSONBody defines parameters for DMAPICreateTaskTemplate.
type DMAPICreateTaskTemplateJSONBody Task

// DMAPIImportTaskTemplateJSONBody defines parameters for DMAPIImportTaskTemplate.
type DMAPIImportTaskTemplateJSONBody TaskTemplateRequest

// DMAPIDeleteTaskParams defines parameters for DMAPIDeleteTask.
type DMAPIDeleteTaskParams struct {
	// force stop task even if some subtask is running
	Force *bool `json:"force,omitempty"`
}

// DMAPIGetTaskParams defines parameters for DMAPIGetTask.
type DMAPIGetTaskParams struct {
	// get task with status
	WithStatus *bool `json:"with_status,omitempty"`
}

// DMAPIUpdateTaskJSONBody defines parameters for DMAPIUpdateTask.
type DMAPIUpdateTaskJSONBody UpdateTaskRequest

// DMAPIGetTaskMigrateTargetsParams defines parameters for DMAPIGetTaskMigrateTargets.
type DMAPIGetTaskMigrateTargetsParams struct {
	SchemaPattern *string `json:"schema_pattern,omitempty"`
	TablePattern  *string `json:"table_pattern,omitempty"`
}

// DMAPIOperateTableStructureJSONBody defines parameters for DMAPIOperateTableStructure.
type DMAPIOperateTableStructureJSONBody OperateTaskTableStructureRequest

// DMAPIStartTaskJSONBody defines parameters for DMAPIStartTask.
type DMAPIStartTaskJSONBody StartTaskRequest

// DMAPIGetTaskStatusParams defines parameters for DMAPIGetTaskStatus.
type DMAPIGetTaskStatusParams struct {
	// source name list
	SourceNameList *SourceNameList `json:"source_name_list,omitempty"`
}

// DMAPIStopTaskJSONBody defines parameters for DMAPIStopTask.
type DMAPIStopTaskJSONBody StopTaskRequest

// DMAPIUpdateClusterInfoJSONRequestBody defines body for DMAPIUpdateClusterInfo for application/json ContentType.
type DMAPIUpdateClusterInfoJSONRequestBody DMAPIUpdateClusterInfoJSONBody

// DMAPICreateSourceJSONRequestBody defines body for DMAPICreateSource for application/json ContentType.
type DMAPICreateSourceJSONRequestBody DMAPICreateSourceJSONBody

// DMAPIUpdateSourceJSONRequestBody defines body for DMAPIUpdateSource for application/json ContentType.
type DMAPIUpdateSourceJSONRequestBody DMAPIUpdateSourceJSONBody

// DMAPIDisableRelayJSONRequestBody defines body for DMAPIDisableRelay for application/json ContentType.
type DMAPIDisableRelayJSONRequestBody DMAPIDisableRelayJSONBody

// DMAPIEnableRelayJSONRequestBody defines body for DMAPIEnableRelay for application/json ContentType.
type DMAPIEnableRelayJSONRequestBody DMAPIEnableRelayJSONBody

// DMAPIPurgeRelayJSONRequestBody defines body for DMAPIPurgeRelay for application/json ContentType.
type DMAPIPurgeRelayJSONRequestBody DMAPIPurgeRelayJSONBody

// DMAPITransferSourceJSONRequestBody defines body for DMAPITransferSource for application/json ContentType.
type DMAPITransferSourceJSONRequestBody DMAPITransferSourceJSONBody

// DMAPICreateTaskJSONRequestBody defines body for DMAPICreateTask for application/json ContentType.
type DMAPICreateTaskJSONRequestBody DMAPICreateTaskJSONBody

// DMAPIConvertTaskJSONRequestBody defines body for DMAPIConvertTask for application/json ContentType.
type DMAPIConvertTaskJSONRequestBody DMAPIConvertTaskJSONBody

// DMAPICreateTaskTemplateJSONRequestBody defines body for DMAPICreateTaskTemplate for application/json ContentType.
type DMAPICreateTaskTemplateJSONRequestBody DMAPICreateTaskTemplateJSONBody

// DMAPIImportTaskTemplateJSONRequestBody defines body for DMAPIImportTaskTemplate for application/json ContentType.
type DMAPIImportTaskTemplateJSONRequestBody DMAPIImportTaskTemplateJSONBody

// DMAPIUpdateTaskJSONRequestBody defines body for DMAPIUpdateTask for application/json ContentType.
type DMAPIUpdateTaskJSONRequestBody DMAPIUpdateTaskJSONBody

// DMAPIOperateTableStructureJSONRequestBody defines body for DMAPIOperateTableStructure for application/json ContentType.
type DMAPIOperateTableStructureJSONRequestBody DMAPIOperateTableStructureJSONBody

// DMAPIStartTaskJSONRequestBody defines body for DMAPIStartTask for application/json ContentType.
type DMAPIStartTaskJSONRequestBody DMAPIStartTaskJSONBody

// DMAPIStopTaskJSONRequestBody defines body for DMAPIStopTask for application/json ContentType.
type DMAPIStopTaskJSONRequestBody DMAPIStopTaskJSONBody

// Getter for additional properties for Task_BinlogFilterRule. Returns the specified
// element and whether it was found
func (a Task_BinlogFilterRule) Get(fieldName string) (value TaskBinLogFilterRule, found bool) {
	if a.AdditionalProperties != nil {
		value, found = a.AdditionalProperties[fieldName]
	}
	return
}

// Setter for additional properties for Task_BinlogFilterRule
func (a *Task_BinlogFilterRule) Set(fieldName string, value TaskBinLogFilterRule) {
	if a.AdditionalProperties == nil {
		a.AdditionalProperties = make(map[string]TaskBinLogFilterRule)
	}
	a.AdditionalProperties[fieldName] = value
}

// Override default JSON handling for Task_BinlogFilterRule to handle AdditionalProperties
func (a *Task_BinlogFilterRule) UnmarshalJSON(b []byte) error {
	object := make(map[string]json.RawMessage)
	err := json.Unmarshal(b, &object)
	if err != nil {
		return err
	}

	if len(object) != 0 {
		a.AdditionalProperties = make(map[string]TaskBinLogFilterRule)
		for fieldName, fieldBuf := range object {
			var fieldVal TaskBinLogFilterRule
			err := json.Unmarshal(fieldBuf, &fieldVal)
			if err != nil {
				return fmt.Errorf("error unmarshaling field %s: %w", fieldName, err)
			}
			a.AdditionalProperties[fieldName] = fieldVal
		}
	}
	return nil
}

// Override default JSON handling for Task_BinlogFilterRule to handle AdditionalProperties
func (a Task_BinlogFilterRule) MarshalJSON() ([]byte, error) {
	var err error
	object := make(map[string]json.RawMessage)

	for fieldName, field := range a.AdditionalProperties {
		object[fieldName], err = json.Marshal(field)
		if err != nil {
			return nil, fmt.Errorf("error marshaling '%s': %w", fieldName, err)
		}
	}
	return json.Marshal(object)
}
