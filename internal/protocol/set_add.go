// Copyright (c) 2008-2018, Hazelcast, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

type SetAddResponseParameters struct {
	Response bool
}

func SetAddCalculateSize(name *string, value *Data) int {
	// Calculates the request payload size
	dataSize := 0
	dataSize += StringCalculateSize(name)
	dataSize += DataCalculateSize(value)
	return dataSize
}

func SetAddEncodeRequest(name *string, value *Data) *ClientMessage {
	// Encode request into clientMessage
	clientMessage := NewClientMessage(nil, SetAddCalculateSize(name, value))
	clientMessage.SetMessageType(SET_ADD)
	clientMessage.IsRetryable = false
	clientMessage.AppendString(name)
	clientMessage.AppendData(value)
	clientMessage.UpdateFrameLength()
	return clientMessage
}

func SetAddDecodeResponse(clientMessage *ClientMessage) *SetAddResponseParameters {
	// Decode response from client message
	parameters := new(SetAddResponseParameters)
	parameters.Response = clientMessage.ReadBool()
	return parameters
}
