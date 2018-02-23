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

type ListAddListenerResponseParameters struct {
	Response *string
}

func ListAddListenerCalculateSize(name *string, includeValue bool, localOnly bool) int {
	// Calculates the request payload size
	dataSize := 0
	dataSize += StringCalculateSize(name)
	dataSize += BOOL_SIZE_IN_BYTES
	dataSize += BOOL_SIZE_IN_BYTES
	return dataSize
}

func ListAddListenerEncodeRequest(name *string, includeValue bool, localOnly bool) *ClientMessage {
	// Encode request into clientMessage
	clientMessage := NewClientMessage(nil, ListAddListenerCalculateSize(name, includeValue, localOnly))
	clientMessage.SetMessageType(LIST_ADDLISTENER)
	clientMessage.IsRetryable = false
	clientMessage.AppendString(name)
	clientMessage.AppendBool(includeValue)
	clientMessage.AppendBool(localOnly)
	clientMessage.UpdateFrameLength()
	return clientMessage
}

func ListAddListenerDecodeResponse(clientMessage *ClientMessage) *ListAddListenerResponseParameters {
	// Decode response from client message
	parameters := new(ListAddListenerResponseParameters)
	parameters.Response = clientMessage.ReadString()
	return parameters
}

func ListAddListenerHandle(clientMessage *ClientMessage, handleEventItem func(*Data, *string, int32)) {
	// Event handler
	messageType := clientMessage.MessageType()
	if messageType == EVENT_ITEM && handleEventItem != nil {
		var item *Data
		if !clientMessage.ReadBool() {
			item = clientMessage.ReadData()
		}
		uuid := clientMessage.ReadString()
		eventType := clientMessage.ReadInt32()
		handleEventItem(item, uuid, eventType)
	}
}
