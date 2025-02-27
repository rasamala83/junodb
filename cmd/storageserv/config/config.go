//
//  Copyright 2023 PayPal Inc.
//
//  Licensed to the Apache Software Foundation (ASF) under one or more
//  contributor license agreements.  See the NOTICE file distributed with
//  this work for additional information regarding copyright ownership.
//  The ASF licenses this file to You under the Apache License, Version 2.0
//  (the "License"); you may not use this file except in compliance with
//  the License.  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/paypal/junodb/third_party/forked/golang/glog"

	"github.com/BurntSushi/toml"

	dbscan "github.com/paypal/junodb/cmd/dbscanserv/config"
	"github.com/paypal/junodb/cmd/storageserv/redist"
	"github.com/paypal/junodb/cmd/storageserv/storage/db"
	"github.com/paypal/junodb/pkg/cluster"
	"github.com/paypal/junodb/pkg/etcd"
	"github.com/paypal/junodb/pkg/initmgr"
	"github.com/paypal/junodb/pkg/io"
	cal "github.com/paypal/junodb/pkg/logging/cal/config"
	otel "github.com/paypal/junodb/pkg/logging/otel/config"
	"github.com/paypal/junodb/pkg/service"
	"github.com/paypal/junodb/pkg/shard"
	"github.com/paypal/junodb/pkg/util"
	"github.com/paypal/junodb/pkg/version"
)

var Initializer initmgr.IInitializer = initmgr.NewInitializer(initialize, finalize)

type Config struct {
	service.Config

	RootDir     string
	StateLogDir string
	PidFileName string
	LogLevel    string
	ClusterName string
	HttpMonAddr string

	StateLogEnabled    bool
	EtcdEnabled        bool
	MicroShardsEnabled bool
	ShardIdValidation  bool
	CloudEnabled       bool
	DbWatchEnabled     bool

	MaxConcurrentRequests uint32
	NumPrefixDbs          uint32
	NumMicroShards        uint32
	NumMicroShardGroups   uint32
	ReqProcCtxPoolSize    uint32
	MaxTimeToLive         uint32

	RecLockExpiration   util.Duration
	ClusterInfo         *cluster.Config
	DB                  *db.Config
	Redist              *redist.Config
	Cal                 cal.Config
	Etcd                etcd.Config
	ShardMapUpdateDelay util.Duration
	OTEL                otel.Config
	DbScan              dbscan.DbScan
}

var serverConfig = Config{
	Config: service.Config{
		ShutdownWaitTime: util.Duration{1 * time.Second},
		IO: io.InboundConfigMap{
			service.DefaultListenerName: io.InboundConfig{
				IdleTimeout:          util.Duration{math.MaxUint32 * time.Second},
				ReadTimeout:          util.Duration{math.MaxUint32 * time.Millisecond},
				WriteTimeout:         util.Duration{math.MaxUint32 * time.Millisecond},
				MaxBufferedWriteSize: 64 * 1024, // default 64k
				RequestTimeout:       util.Duration{600 * time.Millisecond},
				IOBufSize:            64 * 1024,
			},
		},
	},

	LogLevel:              "info",
	PidFileName:           "ss.pid",
	ClusterName:           "cluster",
	EtcdEnabled:           false,
	MaxConcurrentRequests: 3000,
	NumPrefixDbs:          1,
	RecLockExpiration:     util.Duration{600 * time.Millisecond},

	ClusterInfo: &cluster.ClusterInfo[0].Config,

	MicroShardsEnabled:  true,
	NumMicroShards:      0,
	NumMicroShardGroups: 0,

	DB:     &db.DBConfig,
	Redist: &redist.RedistConfig,

	Cal: cal.Config{
		Host:             "127.0.0.1",
		Port:             1118,
		Environment:      "PayPal",
		Poolname:         "junostorageserv",
		MessageQueueSize: 10000,
		CalType:          "socket",
		LogLevel:         "info",
	},
	Etcd:                *etcd.NewConfig("127.0.0.1:2379"),
	ShardIdValidation:   true,
	ShardMapUpdateDelay: util.Duration{30 * time.Second}, // 30 seconds
	ReqProcCtxPoolSize:  10000,
	MaxTimeToLive:       3600 * 24 * 3,
	OTEL: otel.Config{
		Host:        "127.0.0.1",
		Port:        4318,
		Environment: "PayPal",
		Poolname:    "junoproxy",
	},
}

func LoadConfig(ssConfigFile string) (err error) {
	if _, err = toml.DecodeFile(ssConfigFile, &serverConfig); err != nil {
		return
	}
	if err = serverConfig.validatePathAndFileNames(); err != nil {
		return
	}
	if serverConfig.EtcdEnabled {
		etcd.Connect(&serverConfig.Etcd, serverConfig.ClusterName)
		rw := etcd.GetClsReadWriter()

		if rw != nil {
			cluster.Version, err = cluster.ClusterInfo[0].ReadWithRedistInfo(rw)
		}
		if rw == nil || err != nil {
			if cluster.Version, err = cluster.ClusterInfo[0].ReadFromCache(serverConfig.Etcd.CacheName); err == nil {
				glog.Infof("Read etcd cache.")
			}
		}
	} else {
		err = cluster.ClusterInfo[0].PopulateFromConfig()
	}

	if !serverConfig.MicroShardsEnabled && len(serverConfig.DB.DbPaths) >= 1 {
		tagFile := fmt.Sprintf("%s/microshard_enabled.txt", serverConfig.DB.DbPaths[0].Path)
		_, err := os.Stat(tagFile)
		if !errors.Is(err, fs.ErrNotExist) {
			// db was converted by dbcopy tool.
			serverConfig.MicroShardsEnabled = true
		}
	}

	if serverConfig.MicroShardsEnabled {
		if serverConfig.NumMicroShardGroups == 0 || serverConfig.NumMicroShardGroups > 256 {
			serverConfig.NumMicroShardGroups = 8 //default
		}

		if serverConfig.NumMicroShards == 0 || serverConfig.NumMicroShards > 256 {
			serverConfig.NumMicroShards = 256 //default
		}
	} else {
		serverConfig.NumMicroShards = 0
		serverConfig.NumMicroShardGroups = 0
	}

	if err != nil {
		return
	}
	serverConfig.Cal.Label = version.OnelineVersionString()
	if err = serverConfig.Validate(); err == nil {
		serverConfig.OnLoad()
	}

	return
}

func ServerConfig() *Config {
	return &serverConfig
}

func (c *Config) validatePathAndFileNames() (err error) {
	if len(serverConfig.RootDir) == 0 {
		serverConfig.RootDir = filepath.Dir(os.Args[0])
	}
	serverConfig.validatePath(&serverConfig.Etcd.CacheDir)
	serverConfig.validatePath(&serverConfig.StateLogDir)
	if len(serverConfig.PidFileName) == 0 {
		serverConfig.PidFileName = "ss.pid"
	}
	serverConfig.validatePath(&serverConfig.PidFileName)
	for i := 0; i < len(serverConfig.DB.DbPaths); i++ {
		serverConfig.validatePath(&serverConfig.DB.DbPaths[i].Path)
	}
	return
}

// set path to be under Config.RootDir if path is empty or not specified as absolute path
func (c *Config) validatePath(path *string) {
	if path != nil {
		if len(*path) == 0 {
			*path = filepath.Clean(c.RootDir + "/")
		} else if !filepath.IsAbs(*path) {
			*path = filepath.Clean(c.RootDir + "/" + *path)
		}
	}
}

func (c *Config) Validate() (err error) {
	//	if err = c.Config.Validate(); err != nil {
	//		return
	//	}

	if err = c.ClusterInfo.Validate(); err != nil {
		return
	}

	if c.Redist.SnapshotRateLimit == 0 {
		err = fmt.Errorf("Rate limit can't be 0: %d", serverConfig.Redist.SnapshotRateLimit)
		return
	}
	err = c.DB.Validate()

	return
}

func (c *Config) Dump() {
	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	encoder.Encode(c)
	glog.Info(buf.String())
}

// Calculate & set the derived info for once
func (c *Config) NewShardMap(zoneId int, machineId int) (shardMap shard.Map) {

	// TODO: validate the rackid and machineid
	node := cluster.ClusterInfo[0].Zones[zoneId].Nodes[machineId]
	shards := node.GetShards()

	//	if err != nil {
	//		glog.Fatalf("Error getting buckets from shard map:%s", err)
	//	}

	shardMap = shard.NewMapWithSize(len(shards))
	for _, s := range shards {
		shardMap[shard.ID(s)] = struct{}{}
	}
	glog.Verbosef("ShardMap: %v", shardMap)
	return
}

func (c *Config) OnLoad() {
	c.DB.OnLoad()
}

func initialize(args ...interface{}) (err error) {
	sz := len(args)
	if sz < 1 {
		err = fmt.Errorf("a string config file name argument expected")
		return
	}
	filename, ok := args[0].(string)

	if ok == false {
		err = fmt.Errorf("wrong argument type. a string config file name expected")
		return
	}
	err = LoadConfig(filename)
	return
}

func finalize() {
	if serverConfig.EtcdEnabled {
		etcd.Close()
	}
}
