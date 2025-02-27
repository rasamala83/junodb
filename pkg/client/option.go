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

// Package client provides functionalities for client configurations.
package client

import ()

// optionData struct contains client options.
type optionData struct {
	ttl           uint32  // Time to live value.
	context       IContext  // Client context.
	correlationId string  // Correlation ID for tracking.
}

// IOption type represents a function that applies options on optionData.
//type IOption interface {
//	Apply(data *optionData) error
//}
//
//type ApplyOptionFunc func(data *optionData) error
//
//func (f ApplyOptionFunc) Apply(data *optionData) error {
//	return f(data)
//}
//
//func WithTimeToLive(ttl uint32) IOption {
//	return ApplyOptionFunc(func(data *optionData) error {
//		data.ttl = ttl
//		return nil
//	}
//}

type IOption func(data interface{})

// WithTTL function returns an IOption that sets a TTL value.
func WithTTL(ttl uint32) IOption {
	return func(i interface{}) {
		// Check if the passed interface can be casted to *optionData
		if data, ok := i.(*optionData); ok {
			data.ttl = ttl  // Set the TTL value.
		}
	}
}

// WithCond function returns an IOption that sets a context.
func WithCond(context IContext) IOption {
	return func(i interface{}) {
		// Check if the passed interface can be casted to *optionData
		if data, ok := i.(*optionData); ok {
			data.context = context  // Set the context.
		}
	}
}

// WithCorrelationId function returns an IOption that sets a correlationId.
func WithCorrelationId(id string) IOption {
	return func(i interface{}) {
		// Check if the passed interface can be casted to *optionData
		if data, ok := i.(*optionData); ok {
			data.correlationId = id  // Set the correlation ID.
		}
	}
}

// newOptionData function applies the options passed in and returns an initialized optionData.
func newOptionData(opts ...IOption) *optionData {
	data := &optionData{}  // Initialize a new optionData.
	for _, op := range opts {
		op(data)  // Apply each option on the optionData.
	}
	return data  // Return the initialized optionData.
}
