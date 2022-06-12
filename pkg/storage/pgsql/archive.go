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
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"
	kitlog "github.com/go-kit/log"
	"github.com/jackal-xmpp/stravaganza/jid"
	archivemodel "github.com/ortuman/jackal/pkg/model/archive"
)

const (
	archiveTableName = "archives"

	archiveStampFormat = "2006-01-02T15:04:05Z"
)

type pgSQLArchiveRep struct {
	conn   conn
	logger kitlog.Logger
}

func (r *pgSQLArchiveRep) InsertArchiveMessage(ctx context.Context, message *archivemodel.Message) error {
	b, err := message.MarshalBinary()
	if err != nil {
		return err
	}
	fromJID, _ := jid.NewWithString(message.FromJid, true)
	toJID, _ := jid.NewWithString(message.ToJid, true)

	q := sq.Insert(archiveTableName).
		Prefix(noLoadBalancePrefix).
		Columns("archive_id", "id", `"from"`, "from_bare", `"to"`, "to_bare", "message").
		Values(
			message.ArchiveId,
			message.Id,
			fromJID.String(),
			fromJID.ToBareJID().String(),
			toJID.String(),
			toJID.ToBareJID().String(),
			b,
		)

	_, err = q.RunWith(r.conn).ExecContext(ctx)
	return err
}

func (r *pgSQLArchiveRep) FetchArchiveMetadata(ctx context.Context, archiveID string) (*archivemodel.Metadata, error) {
	fromExpr := `FROM `
	fromExpr += `(SELECT "id", created_at FROM archives WHERE serial = (SELECT MIN(serial) FROM archives WHERE archive_id = $1)) AS min,`
	fromExpr += `(SELECT "id", created_at FROM archives WHERE serial = (SELECT MAX(serial) FROM archives WHERE archive_id = $1)) AS max`

	q := sq.Select("min.id, min.created_at, max.id, max.created_at").Suffix(fromExpr, archiveID)

	var start, end time.Time
	var metadata archivemodel.Metadata

	err := q.RunWith(r.conn).
		QueryRowContext(ctx).
		Scan(
			&metadata.StartId,
			&start,
			&metadata.EndId,
			&end,
		)

	switch err {
	case nil:
		metadata.StartTimestamp = start.UTC().Format(archiveStampFormat)
		metadata.EndTimestamp = end.UTC().Format(archiveStampFormat)
		return &metadata, nil

	case sql.ErrNoRows:
		return nil, nil

	default:
		return nil, err
	}
}

func (r *pgSQLArchiveRep) DeleteArchiveOldestMessages(ctx context.Context, archiveID string, maxElements int) error {
	q := sq.Delete(archiveTableName).
		Prefix(noLoadBalancePrefix).
		Where(sq.And{
			sq.Eq{"archive_id": archiveID},
			sq.Expr(`"id" NOT IN (SELECT "id" FROM archives WHERE archive_id = $2 ORDER BY created_at DESC LIMIT $3 OFFSET 0)`, archiveID, maxElements),
		})
	_, err := q.RunWith(r.conn).ExecContext(ctx)
	return err
}

func (r *pgSQLArchiveRep) DeleteArchive(ctx context.Context, archiveID string) error {
	q := sq.Delete(archiveTableName).
		Prefix(noLoadBalancePrefix).
		Where(sq.Eq{"archive_id": archiveID})
	_, err := q.RunWith(r.conn).ExecContext(ctx)
	return err
}
