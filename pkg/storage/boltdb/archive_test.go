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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	archivemodel "github.com/ortuman/jackal/pkg/model/archive"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestBoltDB_InsertArchiveMessage(t *testing.T) {
	t.Parallel()

	db := setupDB(t)
	t.Cleanup(func() { cleanUp(db) })

	err := db.Update(func(tx *bolt.Tx) error {
		rep := boltDBArchiveRep{tx: tx}

		m0 := testMessageStanza("message 0")

		err := rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Message:   m0.Proto(),
		})
		require.NoError(t, err)

		return nil
	})
	require.NoError(t, err)
}

func TestBoltDB_FetchArchiveMetadata(t *testing.T) {
	t.Parallel()

	db := setupDB(t)
	t.Cleanup(func() { cleanUp(db) })

	err := db.Update(func(tx *bolt.Tx) error {
		rep := boltDBArchiveRep{tx: tx}

		m0 := testMessageStanza("message 0")
		m1 := testMessageStanza("message 1")
		m2 := testMessageStanza("message 2")

		now0 := time.Now()
		now1 := now0.Add(time.Hour)
		now2 := now1.Add(time.Hour)

		err := rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Id:        "id0",
			Message:   m0.Proto(),
			Stamp:     timestamppb.New(now0),
		})
		require.NoError(t, err)

		err = rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Id:        "id1",
			Message:   m1.Proto(),
			Stamp:     timestamppb.New(now1),
		})
		require.NoError(t, err)

		err = rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Id:        "id2",
			Message:   m2.Proto(),
			Stamp:     timestamppb.New(now2),
		})
		require.NoError(t, err)

		metadata, err := rep.FetchArchiveMetadata(context.Background(), "a1234")
		require.NoError(t, err)

		require.Equal(t, "id0", metadata.StartId)
		require.Equal(t, now0.UTC().Format(archiveStampFormat), metadata.StartTimestamp)
		require.Equal(t, "id2", metadata.EndId)
		require.Equal(t, now2.UTC().Format(archiveStampFormat), metadata.EndTimestamp)

		return nil
	})
	require.NoError(t, err)
}

func TestBoltDB_DeleteArchiveOldestMessages(t *testing.T) {
	t.Parallel()

	db := setupDB(t)
	t.Cleanup(func() { cleanUp(db) })

	err := db.Update(func(tx *bolt.Tx) error {
		rep := boltDBArchiveRep{tx: tx}

		m0 := testMessageStanza("message 0")
		m1 := testMessageStanza("message 1")
		m2 := testMessageStanza("message 2")

		err := rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Message:   m0.Proto(),
		})
		require.NoError(t, err)

		err = rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Message:   m1.Proto(),
		})
		require.NoError(t, err)

		err = rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Message:   m2.Proto(),
		})
		require.NoError(t, err)

		require.Equal(t, 3, countBucketElements(t, tx, archiveBucket("a1234")))

		err = rep.DeleteArchiveOldestMessages(context.Background(), "a1234", 2)
		require.NoError(t, err)

		require.Equal(t, 2, countBucketElements(t, tx, archiveBucket("a1234")))

		return nil
	})
	require.NoError(t, err)
}

func TestBoltDB_DeleteArchive(t *testing.T) {
	t.Parallel()

	db := setupDB(t)
	t.Cleanup(func() { cleanUp(db) })

	err := db.Update(func(tx *bolt.Tx) error {
		rep := boltDBArchiveRep{tx: tx}

		m0 := testMessageStanza("message 0")
		m1 := testMessageStanza("message 1")

		err := rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Message:   m0.Proto(),
		})
		require.NoError(t, err)

		err = rep.InsertArchiveMessage(context.Background(), &archivemodel.Message{
			ArchiveId: "a1234",
			Message:   m1.Proto(),
		})
		require.NoError(t, err)

		require.Equal(t, 2, countBucketElements(t, tx, archiveBucket("a1234")))

		err = rep.DeleteArchive(context.Background(), "a1234")
		require.NoError(t, err)

		require.Equal(t, 0, countBucketElements(t, tx, archiveBucket("a1234")))

		return nil
	})
	require.NoError(t, err)
}
