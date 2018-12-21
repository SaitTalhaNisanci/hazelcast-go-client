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
	"time"

	"github.com/hazelcast/hazelcast-go-client/internal/nearcache"
	"github.com/hazelcast/hazelcast-go-client/serialization"
)

type NearCachedMapProxy struct {
	*mapProxy
	nearCache        nearcache.NearCache
	serializeKeys    bool
	repairingHandler nearcache.RepairingHandler
}

func newNearCachedMapProxy(client *HazelcastClient, serviceName string, name string) (*NearCachedMapProxy, error) {
	mapProxy, err := newMapProxy(client, serviceName, name)
	nearCachedProxy := &NearCachedMapProxy{
		mapProxy: mapProxy,
	}
	nearCachedProxy.init()
	return nearCachedProxy, err
}

func (n *NearCachedMapProxy) init() {
	nearCacheCfg := n.client.Config.NearCacheConfig()
	n.serializeKeys = nearCacheCfg.IsSerializeKeys()
	nearCacheManager := n.client.nearcacheManager
	n.nearCache = nearCacheManager.GetOrCreateNearCache(n.Name(), nearCacheCfg)

	if nearCacheCfg.InvalidateOnChange() {
		// TODO:: registerInvalidationListener

	}

}

func (n *NearCachedMapProxy) cachedValue(key interface{}) interface{} {
	return n.nearCache.Get(key)
}

func (n *NearCachedMapProxy) ContainsKey(key interface{}) (bool, error) {
	if cachedValue := n.cachedValue(key); cachedValue != nil {
		return true, nil
	}
	return n.mapProxy.ContainsKey(key)
}

func (n *NearCachedMapProxy) Get(key interface{}) (interface{}, error) {
	nearCacheKey, err := n.toNearCacheKey(key)
	if err != nil {
		return nil, err
	}
	if cachedValue := n.cachedValueInObject(nearCacheKey); cachedValue != nil {
		return cachedValue, nil
	}

	value, err := n.mapProxy.Get(nearCacheKey)
	if err != nil {
		return nil, err
	}

	if value != nil {
		return n.tryReserveAndPublishReserved(nearCacheKey, value)
	}

	return value, nil
}

func (n *NearCachedMapProxy) tryReserveAndPublishReserved(nearCacheKey interface{}, value interface{}) (interface{}, error) {
	keyData, err := n.toData(nearCacheKey)
	if err != nil {
		return nil, err
	}
	reservationID, reserved := n.nearCache.TryReserveForUpdate(nearCacheKey, keyData)
	if reserved {
		value, _ = n.tryPublishReserved(nearCacheKey, value, reservationID)
	}
	return value, nil
}

func (n *NearCachedMapProxy) GetAll(keys []interface{}) (map[interface{}]interface{}, error) {
	cachedKeyValues, notCachedKeys := n.populateResultFromNearCache(keys)
	reservations := n.tryReservingKeys(notCachedKeys)
	notCachedKeyValues, err := n.mapProxy.GetAll(notCachedKeys)
	if err != nil {
		n.releaseRemainingReservedKeys(reservations)
		return nil, err
	}
	n.populateResultFromRemote(notCachedKeyValues, reservations)
	for cachedKey, cachedValue := range cachedKeyValues {
		notCachedKeyValues[cachedKey] = cachedValue
	}
	n.releaseRemainingReservedKeys(reservations)
	return notCachedKeyValues, nil
}

func (n *NearCachedMapProxy) Clear() error {
	defer n.nearCache.Clear()
	return n.mapProxy.Clear()
}

func (n *NearCachedMapProxy) EvictAll() error {
	defer n.nearCache.Clear()
	return n.mapProxy.EvictAll()
}

func (n *NearCachedMapProxy) Replace(key, value interface{}) (interface{}, error) {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.Replace(key, value)
}

func (n *NearCachedMapProxy) ReplaceIfSame(key, value, oldValue interface{}) (bool, error) {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.ReplaceIfSame(key, value, oldValue)
}

func (n *NearCachedMapProxy) Set(key, value interface{}) error {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.Set(key, value)
}

func (n *NearCachedMapProxy) SetWithTTL(key, value interface{}, ttl time.Duration) error {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.SetWithTTL(key, value, ttl)
}

func (n *NearCachedMapProxy) PutIfAbsent(key, value interface{}) (interface{}, error) {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.PutIfAbsent(key, value)
}

func (n *NearCachedMapProxy) PutAll(entries map[interface{}]interface{}) error {
	defer n.invalidateAllFromMap(entries)
	return n.mapProxy.PutAll(entries)
}

func (n *NearCachedMapProxy) PutTransient(key, value interface{}, duration time.Duration) error {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.PutTransient(key, value, duration)
}

func (n *NearCachedMapProxy) ExecuteOnKey(key interface{}, entryProcessor interface{}) (interface{}, error) {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.ExecuteOnKey(key, entryProcessor)
}

func (n *NearCachedMapProxy) invalidateAllFromMap(entries map[interface{}]interface{}) {
	for key, _ := range entries {
		n.invalidateNearCacheKey(key)
	}
}

func (n *NearCachedMapProxy) populateResultFromRemote(notCachedKeyValues map[interface{}]interface{},
	reservations map[interface{}]int64) {

	for key, value := range notCachedKeyValues {
		if value != nil {
			_, published := n.tryPublishReserved(key, value, reservations[key])
			if published {
				delete(reservations, key)
			}
		}
	}
}

func (n *NearCachedMapProxy) releaseRemainingReservedKeys(reservations map[interface{}]int64) {
	for nearCacheKey := range reservations {
		n.nearCache.Invalidate(nearCacheKey)
	}
}

func (n *NearCachedMapProxy) tryReservingKeys(notCachedKeys []interface{}) map[interface{}]int64 {
	reservations := make(map[interface{}]int64)
	for _, notCachedKey := range notCachedKeys {
		notCachedKeyData, _ := n.toData(notCachedKey)
		reservationID, reserved := n.nearCache.TryReserveForUpdate(notCachedKey, notCachedKeyData)
		if reserved {
			reservations[notCachedKey] = reservationID
		}
	}
	return reservations
}

func (n *NearCachedMapProxy) populateResultFromNearCache(keys []interface{}) (map[interface{}]interface{}, []interface{}) {
	cachedKeyValue := make(map[interface{}]interface{})
	notCachedKeys := keys[:0]
	for _, key := range keys {
		cachedValue := n.cachedValueInObject(key)
		if cachedValue != nil {
			cachedKeyValue[key] = cachedValue
		} else {
			notCachedKeys = append(notCachedKeys, key)
		}
	}
	return cachedKeyValue, notCachedKeys
}

func (n *NearCachedMapProxy) Put(key interface{}, value interface{}) (interface{}, error) {
	nearCacheKey, err := n.toNearCacheKey(key)
	if err != nil {
		return nil, err
	}
	previousValue, err := n.mapProxy.Put(nearCacheKey, value)
	if err != nil {
		return nil, err
	}
	n.invalidateNearCacheKey(nearCacheKey)
	return previousValue, nil
}

func (n *NearCachedMapProxy) Remove(key interface{}) (interface{}, error) {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.Remove(key)
}

func (n *NearCachedMapProxy) RemoveIfSame(key interface{}, value interface{}) (bool, error) {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.RemoveIfSame(key, value)
}

func (n *NearCachedMapProxy) Evict(key interface{}) (bool, error) {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.Evict(key)
}

func (n *NearCachedMapProxy) Delete(key interface{}) error {
	defer n.invalidateNearCacheKey(key)
	return n.mapProxy.Delete(key)
}

func (n *NearCachedMapProxy) TryRemove(key interface{}, duration time.Duration) (bool, error) {
	removed, err := n.mapProxy.TryRemove(key, duration)
	if removed {
		n.invalidateNearCacheKey(key)
	}
	return removed, err
}

func (n *NearCachedMapProxy) TryPut(key, value interface{}) (bool, error) {
	put, err := n.mapProxy.TryPut(key, value)
	if put {
		n.invalidateNearCacheKey(key)
	}
	return put, err
}

func (n *NearCachedMapProxy) TryPutWithTimeout(key, value interface{}, duration time.Duration) (bool, error) {
	put, err := n.mapProxy.TryPutWithTimeout(key, value, duration)
	if put {
		n.invalidateNearCacheKey(key)
	}
	return put, err
}

func (n *NearCachedMapProxy) RemoveAll(predicate interface{}) error {
	defer n.nearCache.Clear()
	return n.mapProxy.RemoveAll(predicate)
}

func (n *NearCachedMapProxy) tryPublishReserved(key, value interface{}, reservationID int64) (interface{}, bool) {
	cachedValue, published := n.nearCache.TryPublishReserved(key, value, reservationID, true)
	if cachedValue != nil {
		return cachedValue, published
	}
	return value, published
}

func (n *NearCachedMapProxy) cachedValueInObject(key interface{}) interface{} {
	value := n.nearCache.Get(key)
	if data, ok := value.(serialization.Data); ok {
		value, _ = n.toObject(data)
	}
	return value
}

func (n *NearCachedMapProxy) toNearCacheKey(key interface{}) (interface{}, error) {
	if n.serializeKeys {
		return n.toData(key)
	}
	return key, nil
}

func (n *NearCachedMapProxy) invalidateNearCacheKey(key interface{}) {
	n.nearCache.Invalidate(key)
}

// For testing
func (n *NearCachedMapProxy) NearCache() nearcache.NearCache {
	return n.nearCache
}