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

package pgsqlrepository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackal-xmpp/stravaganza"
	archivemodel "github.com/ortuman/jackal/pkg/model/archive"
	"github.com/stretchr/testify/require"
)

func TestPgSQLArchive_InsertArchiveMessage(t *testing.T) {
	// given
	b := stravaganza.NewMessageBuilder()
	b.WithAttribute("from", "noelia@jackal.im/yard")
	b.WithAttribute("to", "ortuman@jackal.im/balcony")
	b.WithChild(
		stravaganza.NewBuilder("body").
			WithText("I'll give thee a wind.").
			Build(),
	)
	msg, _ := b.BuildMessage()

	aMsg := &archivemodel.Message{
		ArchiveId: "ortuman",
		Id:        "id1234",
		FromJid:   "ortuman@jackal.im/local",
		ToJid:     "ortuman@jabber.org/remote",
		Message:   msg.Proto(),
	}
	msgBytes, _ := aMsg.MarshalBinary()

	s, mock := newArchiveMock()
	mock.ExpectExec(`INSERT INTO archives \(archive_id,id,"from",from_bare,"to",to_bare,message\) VALUES \(\$1,\$2,\$3,\$4,\$5,\$6,\$7\)`).
		WithArgs("ortuman", "id1234", "ortuman@jackal.im/local", "ortuman@jackal.im", "ortuman@jabber.org/remote", "ortuman@jabber.org", msgBytes).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// when
	err := s.InsertArchiveMessage(context.Background(), aMsg)

	// then
	require.Nil(t, err)
	require.Nil(t, mock.ExpectationsWereMet())
}

func TestPgSQLArchive_FetchArchiveMetadata(t *testing.T) {
	// given
	s, mock := newArchiveMock()
	mock.ExpectQuery(`SELECT min.id, min.created_at, max.id, max.created_at FROM \(SELECT "id", created_at FROM archives WHERE serial = \(SELECT MIN\(serial\) FROM archives WHERE archive_id = \$1\)\) AS min,\(SELECT "id", created_at FROM archives WHERE serial = \(SELECT MAX\(serial\) FROM archives WHERE archive_id = \$1\)\) AS max`).
		WithArgs("ortuman").
		WillReturnRows(
			sqlmock.NewRows([]string{"min.id", "min.created_at", "max.id", "max.created_at"}).AddRow("YWxwaGEg", "2008-08-22T21:09:04Z", "b21lZ2Eg", "2020-04-20T14:34:21Z"),
		)

	// when
	metadata, err := s.FetchArchiveMetadata(context.Background(), "ortuman")

	// then
	require.Nil(t, err)
	require.NotNil(t, metadata)

	require.Equal(t, "YWxwaGEg", metadata.StartId)
	require.Equal(t, "2008-08-22T21:09:04Z", metadata.StartTimestamp)
	require.Equal(t, "b21lZ2Eg", metadata.EndId)
	require.Equal(t, "2020-04-20T14:34:21Z", metadata.EndTimestamp)

	require.Nil(t, mock.ExpectationsWereMet())
}

func TestPgSQLArchive_DeleteArchiveOldestMessages(t *testing.T) {
	// given
	s, mock := newArchiveMock()
	mock.ExpectExec(`DELETE FROM archives WHERE \(archive_id = \$1 AND "id" NOT IN \(SELECT "id" FROM archives WHERE archive_id = \$2 ORDER BY created_at DESC LIMIT \$3 OFFSET 0\)\)`).
		WithArgs("ortuman", "ortuman", 1234).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// when
	err := s.DeleteArchiveOldestMessages(context.Background(), "ortuman", 1234)

	// then
	require.Nil(t, err)
	require.Nil(t, mock.ExpectationsWereMet())
}

func TestPgSQLArchive_DeleteArchive(t *testing.T) {
	// given
	s, mock := newArchiveMock()
	mock.ExpectExec(`DELETE FROM archives WHERE archive_id = \$1`).
		WithArgs("ortuman").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// when
	err := s.DeleteArchive(context.Background(), "ortuman")

	// then
	require.Nil(t, err)
	require.Nil(t, mock.ExpectationsWereMet())
}

func newArchiveMock() (*pgSQLArchiveRep, sqlmock.Sqlmock) {
	s, sqlMock := newPgSQLMock()
	return &pgSQLArchiveRep{conn: s}, sqlMock
}
