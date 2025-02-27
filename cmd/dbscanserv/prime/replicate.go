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

package prime

import (
	"errors"
	"runtime"
	"time"

	"github.com/paypal/junodb/third_party/forked/golang/glog"

	"github.com/paypal/junodb/internal/cli"
	"github.com/paypal/junodb/pkg/io"
	"github.com/paypal/junodb/pkg/proto"
	"github.com/paypal/junodb/pkg/sec"
)

var (
	secConfig *sec.Config
	processor []*cli.Processor
	inChan    = make(chan *proto.OperationalMessage, 1000)
	outChan   = make(chan bool, 1000)
)

func SetSecConfig(cfg *sec.Config) {
	if cfg == nil {
		return
	}

	secConfig = cfg
	err := sec.Initialize(secConfig, sec.KFlagClientTlsEnabled)
	if err != nil {
		glog.Exitf("Sec init err=%s", err)
	}
}

func InitReplicator(proxyAddr string, numConns int) {
	if len(proxyAddr) == 0 {
		return
	}

	if numConns <= 0 {
		numConns = 1
	}
	if numConns > 4 {
		numConns = 4
	}

	processor = make([]*cli.Processor, numConns)
	for i := 0; i < numConns; i++ {

		processor[i] = cli.NewProcessor(
			io.ServiceEndpoint{Addr: proxyAddr, SSLEnabled: secConfig != nil},
			"dbscan",
			time.Duration(500*time.Millisecond),  // ConnectTimeout
			time.Duration(1000*time.Millisecond), // RequestTimeout
			time.Duration(60*time.Second))        // ConnectRecycleTimeout

		processor[i].Start()

		runtime.SetFinalizer(processor[i], func(p *cli.Processor) {
			p.Close()
		})

		go processRequest(i)
	}
}

func processRequest(k int) {
	count := uint64(0)
	for {
		select {
		case op := <-inChan:
			for i := 0; i < 3; i++ {
				reply, err := processor[k].ProcessRequest(op)
				if err != nil {
					if i < 2 {
						time.Sleep(10 * time.Millisecond)
						continue
					}
					count++
					if (count % 1000) < 6 {
						glog.Errorf("Replicate err=%s", err)
					}
					outChan <- false // fail
					break
				}

				err = checkResponse(op, reply)
				if err != nil {
					count++
					if (count % 1000) < 6 {
						glog.Errorf("Replicate err=%s", err)
					}
					outChan <- false // fail
					break
				}

				outChan <- true // success
				break
			}
		}
	}
}

func ReplicateRecord(op *proto.OperationalMessage) bool {
	if op == nil || processor == nil {
		return false
	}

	inChan <- op
	rt := <-outChan
	return rt
}

func checkResponse(request *proto.OperationalMessage,
	response *proto.OperationalMessage) error {

	status := response.GetOpStatus()
	if status == proto.OpStatusNoError ||
		status == proto.OpStatusVersionConflict {
		return nil
	}
	return errors.New(status.String())
}
