// Copyright 2020 PingCAP, Inc.
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

package regionspan

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/google/btree"
	"github.com/pingcap/log"
	"go.uber.org/zap"
)

type rangeTsEntry struct {
	// Only startKey is necessary. End key can be inferred by the next item since the map always keeps a continuous
	// range.
	startKey []byte
	ts       uint64
}

func rangeTsEntryWithKey(key []byte) *rangeTsEntry {
	return &rangeTsEntry{
		startKey: key,
	}
}

func rangeTsEntryLess(a, b *rangeTsEntry) bool {
	return bytes.Compare(a.startKey, b.startKey) < 0
}

// RangeTsMap represents a map from key range to a timestamp. It supports range set and calculating min value among a
// a specified range.
type RangeTsMap struct {
	m *btree.BTreeG[*rangeTsEntry]
}

// NewRangeTsMap creates a RangeTsMap.
func NewRangeTsMap(startKey, endKey []byte, startTs uint64) *RangeTsMap {
	m := &RangeTsMap{
		m: btree.NewG[*rangeTsEntry](16, rangeTsEntryLess),
	}
	m.Set(startKey, endKey, startTs)
	return m
}

// Set sets the corresponding ts of the given range to the specified value.
func (m *RangeTsMap) Set(startKey, endKey []byte, ts uint64) {
	if _, ok := m.m.Get(rangeTsEntryWithKey(endKey)); !ok {
		// To calculate the minimal ts, the default value is math.MaxUint64
		tailTs := uint64(math.MaxUint64)
		m.m.DescendLessOrEqual(rangeTsEntryWithKey(endKey), func(i *rangeTsEntry) bool {
			tailTs = i.ts
			return false
		})
		m.m.ReplaceOrInsert(&rangeTsEntry{
			startKey: endKey,
			ts:       tailTs,
		})
	}

	entriesToDelete := make([]*rangeTsEntry, 0)
	m.m.AscendRange(rangeTsEntryWithKey(startKey), rangeTsEntryWithKey(endKey),
		func(i *rangeTsEntry) bool {
			entriesToDelete = append(entriesToDelete, i)
			return true
		})

	for _, e := range entriesToDelete {
		m.m.Delete(e)
	}

	m.m.ReplaceOrInsert(&rangeTsEntry{
		startKey: startKey,
		ts:       ts,
	})
}

// GetMin gets the min ts value among the given range. endKey must be greater than startKey.
func (m *RangeTsMap) GetMin(startKey, endKey []byte) uint64 {
	var ts uint64 = math.MaxUint64
	m.m.DescendLessOrEqual(rangeTsEntryWithKey(startKey), func(i *rangeTsEntry) bool {
		ts = i.ts
		return false
	})
	m.m.AscendRange(rangeTsEntryWithKey(startKey), rangeTsEntryWithKey(endKey),
		func(i *rangeTsEntry) bool {
			thisTs := i.ts
			if ts > thisTs {
				ts = thisTs
			}
			return true
		})
	return ts
}

type rangeLockEntry struct {
	startKey []byte
	endKey   []byte
	regionID uint64
	version  uint64
	waiters  []chan<- interface{}
}

func rangeLockEntryWithKey(key []byte) *rangeLockEntry {
	return &rangeLockEntry{
		startKey: key,
	}
}

func rangeLockEntryLess(a, b *rangeLockEntry) bool {
	return bytes.Compare(a.startKey, b.startKey) < 0
}

func (e *rangeLockEntry) String() string {
	return fmt.Sprintf("region %v [%v, %v), version %v, %d waiters",
		e.regionID,
		hex.EncodeToString(e.startKey),
		hex.EncodeToString(e.endKey),
		e.version,
		len(e.waiters))
}

var currentID uint64 = 0

func allocID() uint64 {
	return atomic.AddUint64(&currentID, 1)
}

// RegionRangeLock is specifically used for kv client to manage exclusive region ranges. Acquiring lock will be blocked
// if part of its range is already locked. It also manages checkpoint ts of all ranges. The ranges are marked by a
// version number, which should comes from the Region's Epoch version. The version is used to compare which range is
// new and which is old if two ranges are overlapping.
type RegionRangeLock struct {
	changefeedLogInfo string
	mu                sync.Mutex
	rangeCheckpointTs *RangeTsMap
	rangeLock         *btree.BTreeG[*rangeLockEntry]
	regionIDLock      map[uint64]*rangeLockEntry
	// ID to identify different RegionRangeLock instances, so logs of different instances can be distinguished.
	id uint64
}

// NewRegionRangeLock creates a new RegionRangeLock.
func NewRegionRangeLock(
	startKey, endKey []byte, startTs uint64, changefeedLogInfo string,
) *RegionRangeLock {
	return &RegionRangeLock{
		changefeedLogInfo: changefeedLogInfo,
		rangeCheckpointTs: NewRangeTsMap(startKey, endKey, startTs),
		rangeLock:         btree.NewG[*rangeLockEntry](16, rangeLockEntryLess),
		regionIDLock:      make(map[uint64]*rangeLockEntry),
		id:                allocID(),
	}
}

func (l *RegionRangeLock) getOverlappedEntries(startKey, endKey []byte, regionID uint64) []*rangeLockEntry {
	regionIDFound := false

	overlappingRanges := make([]*rangeLockEntry, 0)
	l.rangeLock.DescendLessOrEqual(rangeLockEntryWithKey(startKey),
		func(entry *rangeLockEntry) bool {
			if bytes.Compare(entry.startKey, startKey) < 0 &&
				bytes.Compare(startKey, entry.endKey) < 0 {
				overlappingRanges = append(overlappingRanges, entry)
				if entry.regionID == regionID {
					regionIDFound = true
				}
			}
			return false
		})
	l.rangeLock.AscendRange(rangeLockEntryWithKey(startKey), rangeLockEntryWithKey(endKey),
		func(entry *rangeLockEntry) bool {
			overlappingRanges = append(overlappingRanges, entry)
			if entry.regionID == regionID {
				regionIDFound = true
			}
			return true
		})

	// The entry with the same regionID should also be checked.
	if !regionIDFound {
		entry, ok := l.regionIDLock[regionID]
		if ok {
			overlappingRanges = append(overlappingRanges, entry)
		}
	}

	return overlappingRanges
}

func (l *RegionRangeLock) tryLockRange(startKey, endKey []byte, regionID, version uint64) (LockRangeResult, []<-chan interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	overlappingEntries := l.getOverlappedEntries(startKey, endKey, regionID)

	if len(overlappingEntries) == 0 {
		checkpointTs := l.rangeCheckpointTs.GetMin(startKey, endKey)
		newEntry := &rangeLockEntry{
			startKey: startKey,
			endKey:   endKey,
			regionID: regionID,
			version:  version,
		}
		l.rangeLock.ReplaceOrInsert(newEntry)
		l.regionIDLock[regionID] = newEntry

		log.Info("range locked",
			zap.Uint64("lockID", l.id), zap.Uint64("regionID", regionID),
			zap.Uint64("version", version),
			zap.Uint64("checkpointTs", checkpointTs),
			zap.String("startKey", hex.EncodeToString(startKey)),
			zap.String("endKey", hex.EncodeToString(endKey)))

		return LockRangeResult{
			Status:       LockRangeStatusSuccess,
			CheckpointTs: checkpointTs,
		}, nil
	}

	// Format overlapping ranges for printing log
	var overlapStr []string
	for _, r := range overlappingEntries {
		overlapStr = append(overlapStr, fmt.Sprintf("regionID: %v, ver: %v, start: %v, end: %v",
			r.regionID, r.version, hex.EncodeToString(r.startKey), hex.EncodeToString(r.endKey))) // DEBUG
	}

	isStale := false
	for _, r := range overlappingEntries {
		if r.version >= version {
			isStale = true
			break
		}
	}
	if isStale {
		retryRanges := make([]ComparableSpan, 0)
		currentRangeStartKey := startKey

		log.Info("tryLockRange stale",
			zap.String("changefeed", l.changefeedLogInfo),
			zap.Uint64("lockID", l.id), zap.Uint64("regionID", regionID),
			zap.String("startKey", hex.EncodeToString(startKey)),
			zap.String("endKey", hex.EncodeToString(endKey)),
			zap.Strings("allOverlapping", overlapStr)) // DEBUG

		for _, r := range overlappingEntries {
			// Ignore the totally-disjointed range which may be added to the list because of
			// searching by regionID.
			if bytes.Compare(r.endKey, startKey) <= 0 || bytes.Compare(endKey, r.startKey) <= 0 {
				continue
			}
			// The rest should come from range searching and is sorted in increasing order, and they
			// must intersect with the current given range.
			if bytes.Compare(currentRangeStartKey, r.startKey) < 0 {
				retryRanges = append(retryRanges, ComparableSpan{Start: currentRangeStartKey, End: r.startKey})
			}
			currentRangeStartKey = r.endKey
		}
		if bytes.Compare(currentRangeStartKey, endKey) < 0 {
			retryRanges = append(retryRanges, ComparableSpan{Start: currentRangeStartKey, End: endKey})
		}

		return LockRangeResult{
			Status:      LockRangeStatusStale,
			RetryRanges: retryRanges,
		}, nil
	}

	var signalChs []<-chan interface{}

	for _, r := range overlappingEntries {
		ch := make(chan interface{}, 1)
		signalChs = append(signalChs, ch)
		r.waiters = append(r.waiters, ch)

	}

	log.Info("lock range blocked",
		zap.String("changefeed", l.changefeedLogInfo),
		zap.Uint64("lockID", l.id), zap.Uint64("regionID", regionID),
		zap.String("startKey", hex.EncodeToString(startKey)),
		zap.String("endKey", hex.EncodeToString(endKey)),
		zap.Strings("blockedBy", overlapStr)) // DEBUG

	return LockRangeResult{
		Status: LockRangeStatusWait,
	}, signalChs
}

// LockRange locks a range with specified version.
func (l *RegionRangeLock) LockRange(ctx context.Context, startKey, endKey []byte, regionID, version uint64) LockRangeResult {
	res, signalChs := l.tryLockRange(startKey, endKey, regionID, version)

	if res.Status != LockRangeStatusWait {
		return res
	}

	res.WaitFn = func() LockRangeResult {
		signalChs1 := signalChs
		var res1 LockRangeResult
		for {
			for _, ch := range signalChs1 {
				select {
				case <-ctx.Done():
					return LockRangeResult{Status: LockRangeStatusCancel}
				case <-ch:
				}
			}
			res1, signalChs1 = l.tryLockRange(startKey, endKey, regionID, version)
			if res1.Status != LockRangeStatusWait {
				return res1
			}
		}
	}

	return res
}

// UnlockRange unlocks a range and update checkpointTs of the range to specivied value.
func (l *RegionRangeLock) UnlockRange(startKey, endKey []byte, regionID, version uint64, checkpointTs uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.rangeLock.Get(rangeLockEntryWithKey(startKey))

	if !ok {
		log.Panic("unlocking a not locked range",
			zap.String("changefeed", l.changefeedLogInfo),
			zap.Uint64("regionID", regionID),
			zap.String("startKey", hex.EncodeToString(startKey)),
			zap.String("endKey", hex.EncodeToString(endKey)),
			zap.Uint64("version", version),
			zap.Uint64("checkpointTs", checkpointTs))
	}

	if entry.regionID != regionID {
		log.Panic("unlocked a range but regionID mismatch",
			zap.String("changefeed", l.changefeedLogInfo),
			zap.Uint64("expectedRegionID", regionID),
			zap.Uint64("foundRegionID", entry.regionID),
			zap.String("startKey", hex.EncodeToString(startKey)),
			zap.String("endKey", hex.EncodeToString(endKey)))
	}
	if entry != l.regionIDLock[regionID] {
		log.Panic("range lock and region id lock mismatch when trying to unlock",
			zap.String("changefeed", l.changefeedLogInfo),
			zap.Uint64("unlockingRegionID", regionID),
			zap.String("rangeLockEntry", entry.String()),
			zap.String("regionIDLockEntry", l.regionIDLock[regionID].String()))
	}
	delete(l.regionIDLock, regionID)

	if entry.version != version || !bytes.Equal(entry.endKey, endKey) {
		log.Panic("unlocking region doesn't match the locked region",
			zap.String("changefeed", l.changefeedLogInfo),
			zap.Uint64("regionID", regionID),
			zap.String("startKey", hex.EncodeToString(startKey)),
			zap.String("endKey", hex.EncodeToString(endKey)),
			zap.Uint64("version", version),
			zap.Uint64("checkpointTs", checkpointTs),
			zap.String("foundLockEntry", entry.String()))
	}

	for _, ch := range entry.waiters {
		ch <- nil
	}

	_, ok = l.rangeLock.Delete(entry)
	if !ok {
		panic("unreachable")
	}
	l.rangeCheckpointTs.Set(startKey, endKey, checkpointTs)
	log.Info("unlocked range",
		zap.String("changefeed", l.changefeedLogInfo),
		zap.Uint64("lockID", l.id), zap.Uint64("regionID", entry.regionID),
		zap.Uint64("checkpointTs", checkpointTs),
		zap.String("startKey", hex.EncodeToString(startKey)),
		zap.String("endKey", hex.EncodeToString(endKey)))
}

const (
	// LockRangeStatusSuccess means a LockRange operation succeeded.
	LockRangeStatusSuccess = 0
	// LockRangeStatusWait means a LockRange operation is blocked and should wait for it being finished.
	LockRangeStatusWait = 1
	// LockRangeStatusStale means a LockRange operation is rejected because of the range's version is stale.
	LockRangeStatusStale = 2
	// LockRangeStatusCancel means a LockRange operation is cancelled.
	LockRangeStatusCancel = 3
)

// LockRangeResult represents the result of LockRange method of RegionRangeLock.
// If Status is LockRangeStatusSuccess, the CheckpointTs field will be the minimal checkpoint ts among the locked
// range.
// If Status is LockRangeStatusWait, it means the lock cannot be acquired immediately. WaitFn must be invoked to
// continue waiting and acquiring the lock.
// If Status is LockRangeStatusStale, it means the LockRange request is stale because there's already a overlapping
// locked range, whose version is greater or equals to the requested one.
type LockRangeResult struct {
	Status       int
	CheckpointTs uint64
	WaitFn       func() LockRangeResult
	RetryRanges  []ComparableSpan
}
