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

package model

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	dmysql "github.com/go-sql-driver/mysql"
	"github.com/pingcap/tiflow/engine/pkg/dbutil"
)

const (
	defaultReadTimeout  = "3s"
	defaultWriteTimeout = "3s"
	defaultDialTimeout  = "3s"
)

// StoreType is the type of metastore
type StoreType = string

const (
	defaultStoreType = StoreTypeMySQL
	// StoreTypeEtcd is the store type string for etcd
	StoreTypeEtcd = "etcd"
	// StoreTypeMySQL is the store type string for MySQL
	StoreTypeMySQL = "mysql"

	// StoreTypeSQLite is the store type string for SQLite
	// Only for test now
	StoreTypeSQLite = "sqlite"
	// StoreTypeMockKV is a specific store type which can generate
	// a mock kvclient (using map as backend)
	// Only for test now
	StoreTypeMockKV = "mock-kv"
)

// AuthConfParams is basic authentication configurations
type AuthConfParams struct {
	User   string `toml:"user" json:"user"`
	Passwd string `toml:"passwd" json:"passwd"`
}

// StoreConfig is metastore connection configurations
type StoreConfig struct {
	// StoreID is the unique readable identifier for a store
	StoreID string `toml:"store-id" json:"store-id"`
	// StoreType supports 'etcd' or 'mysql', default is 'mysql'
	StoreType StoreType       `toml:"store-type" json:"store-type"`
	Endpoints []string        `toml:"endpoints" json:"endpoints"`
	Auth      *AuthConfParams `toml:"auth" json:"auth"`
	// Schema is the predefine schema name for mysql-compatible metastore
	// 1.It needs to stay UNCHANGED for one dataflow engine cluster
	// 2.It needs be different between any two dataflow engine clusters
	// 3.Naming rule: https://dev.mysql.com/doc/refman/5.7/en/identifiers.html
	Schema       string `toml:"schema" json:"schema"`
	ReadTimeout  string `toml:"read-timeout" json:"read-timeout"`
	WriteTimeout string `toml:"write-timeout" json:"write-timeout"`
	DialTimeout  string `toml:"dial-timeout" json:"dial-timeout"`
	// DBConf is the db config for mysql-compatible metastore
	DBConf *dbutil.DBConfig `toml:"dbconfs" json:"dbconfs"`
}

// SetEndpoints sets endpoints to StoreConfig
func (s *StoreConfig) SetEndpoints(endpoints string) {
	if endpoints != "" {
		s.Endpoints = strings.Split(endpoints, ",")
	}
}

// Validate implements the validation.Validatable interface
func (s StoreConfig) Validate() error {
	return validation.ValidateStruct(&s,
		validation.Field(&s.StoreType, validation.In(StoreTypeEtcd, StoreTypeMySQL)),
		validation.Field(&s.Schema, validation.When(s.StoreType == StoreTypeMySQL, validation.Required, validation.Length(1, 128))),
	)
}

// DefaultStoreConfig return a default *StoreConfig
func DefaultStoreConfig() *StoreConfig {
	return &StoreConfig{
		StoreType:    defaultStoreType,
		Endpoints:    []string{},
		Auth:         &AuthConfParams{},
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
		DialTimeout:  defaultDialTimeout,
		DBConf:       dbutil.DefaultDBConfig(),
	}
}

// GenerateDSNByParams generates a dsn string.
// dsn format: [username[:password]@][protocol[(address)]]/
func GenerateDSNByParams(storeConf *StoreConfig, pairs map[string]string) string {
	if storeConf == nil {
		return "invalid dsn"
	}

	dsnCfg := dmysql.NewConfig()
	if dsnCfg.Params == nil {
		dsnCfg.Params = make(map[string]string, 1)
	}
	if storeConf.Auth != nil {
		dsnCfg.User = storeConf.Auth.User
		dsnCfg.Passwd = storeConf.Auth.Passwd
	}
	dsnCfg.Net = "tcp"
	dsnCfg.Addr = storeConf.Endpoints[0]
	dsnCfg.DBName = storeConf.Schema
	dsnCfg.InterpolateParams = true
	dsnCfg.Params["parseTime"] = "true"
	// TODO: check for timezone
	dsnCfg.Params["loc"] = "Local"
	dsnCfg.Params["readTimeout"] = storeConf.ReadTimeout
	dsnCfg.Params["writeTimeout"] = storeConf.WriteTimeout
	dsnCfg.Params["timeout"] = storeConf.DialTimeout

	for k, v := range pairs {
		dsnCfg.Params[k] = v
	}

	return dsnCfg.FormatDSN()
}

// ToClientType translates store type to client type
func ToClientType(storeType StoreType) ClientType {
	switch storeType {
	case StoreTypeEtcd:
		return EtcdKVClientType
	case StoreTypeMySQL:
		return SQLKVClientType
	case StoreTypeMockKV:
		return MockKVClientType
	}

	return UnknownKVClientType
}
