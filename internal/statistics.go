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

package internal

import (
	"sync/atomic"

	"log"

	"runtime"
	"strconv"

	"bytes"

	"reflect"

	"time"

	"syscall"

	"github.com/hazelcast/hazelcast-go-client/config/property"
	"github.com/hazelcast/hazelcast-go-client/internal/proto"
	"github.com/hazelcast/hazelcast-go-client/internal/util/timeutil"
	"github.com/hazelcast/hazelcast-go-client/internal/util/versionutil"
)

const (
	sinceVersionString = "3.9"
	statSeperator      = ','
	keyValueSeparator  = '='
)

var sinceVersion = versionutil.CalculateVersion(sinceVersionString)

type statistics struct {
	client       *HazelcastClient
	enabled      bool
	ownerAddress atomic.Value
	gauges       map[string]string
	cancel       chan struct{}
}

func newStatistics(client *HazelcastClient) *statistics {
	stats := statistics{
		client: client,
		cancel: make(chan struct{}),
	}
	stats.enabled = client.properties.GetBoolean(property.StatisticsEnabled)
	stats.gauges = make(map[string]string)
	stats.ownerAddress.Store(&proto.Address{}) // initialize
	return &stats
}

// start registers all client statistics and schedules periodic collection of stats.
func (s *statistics) start() {
	if !s.enabled {
		return
	}
	period := s.client.properties.GetDuration(property.StatisticsPeriodSeconds)
	if period <= 0 {
		// This will return the default value since we have a non-positive value now
		defaultPeriod := s.client.properties.GetPositiveDurationOrDef(property.StatisticsPeriodSeconds)
		log.Println("Provided client statistics ", property.StatisticsPeriodSeconds.Name(),
			" cannot be less than or equal to 0.",
			" You provided ", period, " as the configuration. Client will use the default value ", defaultPeriod,
			" instead.")
		period = defaultPeriod
	}
	go s.schedulePeriodicStatisticsSendTask(period)
}

func (s *statistics) schedulePeriodicStatisticsSendTask(period time.Duration) {
	s.sendStatistics()
	ticker := time.NewTicker(period)
	for {
		select {
		case <-s.cancel:
			return
		case <-ticker.C:
			s.sendStatistics()
		}
	}
}

func (s *statistics) getOwnerConnection() *Connection {
	connection := s.client.ConnectionManager.getOwnerConnection()
	if connection == nil {
		return nil
	}
	ownerConnectionAddress := s.client.ClusterService.getOwnerConnectionAddress()
	serverVersion := connection.connectedServerVersion
	if serverVersion < sinceVersion {
		// do not print too many logs if connected to an old version server
		cachedOwnerAddress, ok := s.ownerAddress.Load().(*proto.Address)
		if !ok || (ownerConnectionAddress != cachedOwnerAddress) {
			log.Println("Client statistics cannot be sent to server ", ownerConnectionAddress,
				" since connected server version is less than the minimum supported server version ",
				sinceVersionString)
		}

		// cache the last connected server address for decreasing the log prints
		s.ownerAddress.Store(ownerConnectionAddress)
		return nil
	}
	return connection
}

func (s *statistics) sendStatistics() {
	ownerConnection := s.getOwnerConnection()
	if ownerConnection == nil {
		log.Println("Cannot send client statistics to the server. No owner connection")
		return
	}

	stats := &bytes.Buffer{}

	s.fillMetrics(stats, ownerConnection)
	s.sendStatsToOwner(stats)
}

func (s *statistics) sendStatsToOwner(stats *bytes.Buffer) {
	request := proto.ClientStatisticsEncodeRequest(stats.String())
	ownerConnection := s.getOwnerConnection()
	_, err := s.client.InvocationService.invokeOnConnection(request, ownerConnection).Result()
	if err != nil {
		log.Println("Could not send the statistics, ", err)
	}
}

func (s *statistics) collectMetrics() {
	s.collectOSMetrics()
	s.collectRuntimeMetrics()
}

func (s *statistics) collectRuntimeMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.registerGauge("runtime.availableProcessors", strconv.Itoa(runtime.NumCPU()))
	s.registerGauge("runtime.freeMemory", strconv.Itoa(int(m.HeapIdle)))
	s.registerGauge("runtime.totalMemory", strconv.Itoa(int(m.HeapSys)))
	s.registerGauge("runtime.usedMemory", strconv.Itoa(int(m.HeapInuse)))
}

func (s *statistics) collectOSMetrics() {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err == nil {
		s.registerGauge("os.maxFileDescriptorCount", strconv.Itoa(int(limit.Max)))
		s.registerGauge("os.openFileDescriptorCount", strconv.Itoa(int(limit.Cur)))
	}
}

func (s *statistics) registerGauge(gaugeName string, gaugeValue string) {
	s.gauges[gaugeName] = gaugeValue
}

func (s *statistics) addStat(stats *bytes.Buffer, name string, value interface{}) {
	s.addStatWithKeyPrefix(stats, nil, name, value)
}

func (s *statistics) addStatWithKeyPrefix(stats *bytes.Buffer, keyPrefix *string, name string, value interface{}) {
	if stats.Len() != 0 {
		stats.WriteString(string(statSeperator))
	}

	if keyPrefix != nil {
		stats.WriteString(*keyPrefix)
	}

	stats.WriteString(name)
	stats.WriteString(string(keyValueSeparator))

	switch value.(type) {
	case string:
		stats.WriteString(value.(string))
	case int64:
		stats.WriteString(strconv.Itoa(int(value.(int64))))
	default:
		log.Println("Unexcepted type in addStat: ", reflect.TypeOf(value))
	}
}

func (s *statistics) fillMetrics(stats *bytes.Buffer, ownerConnection *Connection) {
	s.collectMetrics()
	s.addStat(stats, "lastStatisticsCollectionTime", timeutil.GetCurrentTimeInMilliSeconds())
	s.addStat(stats, "enterprise", "false")
	s.addStat(stats, "clientType", "GO")
	s.addStat(stats, "clientVersion", ClientVersion)
	s.addStat(stats, "clusterConnectionTimestamp", ownerConnection.StartTime())
	s.addStat(stats, "clientAddress", ownerConnection.localAddress().String())
	s.addStat(stats, "clientName", s.client.Name())

	s.addStat(stats, "credentials.principal", s.client.Config.GroupConfig().Name())
	for gaugeKey, gaugeValue := range s.gauges {
		s.addStat(stats, gaugeKey, gaugeValue)
	}
}

func (s *statistics) shutdown() {
	close(s.cancel)
}
