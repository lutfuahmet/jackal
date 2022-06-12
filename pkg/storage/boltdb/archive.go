// Copyright 2022 The jackal Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package boltdb

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	archivemodel "github.com/ortuman/jackal/pkg/model/archive"
	bolt "go.etcd.io/bbolt"
)

const archiveStampFormat = "2006-01-02T15:04:05Z"

type boltDBArchiveRep struct {
	tx *bolt.Tx
}

func newArchiveRep(tx *bolt.Tx) *boltDBArchiveRep {
	return &boltDBArchiveRep{tx: tx}
}

func (r *boltDBArchiveRep) InsertArchiveMessage(_ context.Context, message *archivemodel.Message) error {
	op := insertSeqOp{
		tx:     r.tx,
		bucket: archiveBucket(message.ArchiveId),
		obj:    message,
	}
	return op.do()
}

func (r *boltDBArchiveRep) FetchArchiveMetadata(ctx context.Context, archiveID string) (metadata *archivemodel.Metadata, err error) {
	bucketID := archiveBucket(archiveID)

	b := r.tx.Bucket([]byte(bucketID))
	if b == nil {
		return nil, nil
	}
	var retVal archivemodel.Metadata

	c := b.Cursor()
	_, val := c.First()

	var msg archivemodel.Message
	if err := proto.Unmarshal(val, &msg); err != nil {
		return nil, err
	}
	retVal.StartId = msg.Id
	retVal.StartTimestamp = msg.Stamp.AsTime().UTC().Format(archiveStampFormat)

	_, val = c.Last()
	if err := proto.Unmarshal(val, &msg); err != nil {
		return nil, err
	}
	retVal.EndId = msg.Id
	retVal.EndTimestamp = msg.Stamp.AsTime().UTC().Format(archiveStampFormat)

	return &retVal, nil
}

func (r *boltDBArchiveRep) DeleteArchiveOldestMessages(_ context.Context, archiveID string, maxElements int) error {
	bucketID := archiveBucket(archiveID)

	b := r.tx.Bucket([]byte(bucketID))
	if b == nil {
		return nil
	}
	// count items
	var count int

	c := b.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		count++
	}
	if count < maxElements {
		return nil
	}
	// store old value keys
	var oldKeys [][]byte

	c = b.Cursor()
	for k, _ := c.First(); k != nil; k, _ = c.Next() {
		if count <= maxElements {
			break
		}
		count--
		oldKeys = append(oldKeys, k)
	}
	// delete old values
	for _, k := range oldKeys {
		if err := b.Delete(k); err != nil {
			return err
		}
	}
	return nil
}

func (r *boltDBArchiveRep) DeleteArchive(_ context.Context, archiveID string) error {
	op := delBucketOp{
		tx:     r.tx,
		bucket: archiveBucket(archiveID),
	}
	return op.do()
}

func archiveBucket(archiveID string) string {
	return fmt.Sprintf("archive:%s", archiveID)
}

// InsertArchiveMessage inserts a new message element into an archive queue.
func (r *Repository) InsertArchiveMessage(ctx context.Context, message *archivemodel.Message) error {
	return r.db.Update(func(tx *bolt.Tx) error {
		return newArchiveRep(tx).InsertArchiveMessage(ctx, message)
	})
}

// FetchArchiveMetadata returns the metadata value associated to an archive.
func (r *Repository) FetchArchiveMetadata(ctx context.Context, archiveID string) (metadata *archivemodel.Metadata, err error) {
	err = r.db.View(func(tx *bolt.Tx) error {
		metadata, err = newArchiveRep(tx).FetchArchiveMetadata(ctx, archiveID)
		return err
	})
	return
}

// DeleteArchive clears an archive queue.
func (r *Repository) DeleteArchive(ctx context.Context, archiveID string) error {
	return r.db.Update(func(tx *bolt.Tx) error {
		return newArchiveRep(tx).DeleteArchive(ctx, archiveID)
	})
}
