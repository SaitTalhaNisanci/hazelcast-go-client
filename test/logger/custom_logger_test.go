// Copyright (c) 2008-2018, Hazelcast, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License")
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

package logger

import (
	"testing"

	"github.com/hazelcast/hazelcast-go-client"
	"github.com/hazelcast/hazelcast-go-client/test"
	"github.com/stretchr/testify/assert"
)

type customLogger struct {
	logged bool
}

func (c *customLogger) Debug(args ...interface{}) {
}

func (c *customLogger) Trace(args ...interface{}) {
}

func (c *customLogger) Info(args ...interface{}) {
	c.logged = true
}

func (c *customLogger) Warn(args ...interface{}) {
}

func (c *customLogger) Error(args ...interface{}) {
}

func TestCustomLogger(t *testing.T) {
	cluster, _ := remoteController.CreateCluster("", test.DefaultServerConfig)
	defer remoteController.ShutdownCluster(cluster.ID)
	remoteController.StartMember(cluster.ID)
	customLogger := &customLogger{}
	config := hazelcast.NewConfig()
	config.LoggerConfig().SetLogger(customLogger)
	client, err := hazelcast.NewClientWithConfig(config)
	assert.NoError(t, err)
	assert.True(t, customLogger.logged)
	client.Shutdown()
}
