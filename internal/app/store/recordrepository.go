package store

import "github.com/Gorynychdo/aster_go/internal/app/model"

type RecordRepository struct {
	store *Store
}

func (r *RecordRepository) Create(rec *model.Record) error {
    return r.store.db.QueryRow(
        `INSERT INTO records
            (account_id, wav_file_name, creation_time, complete_time, confirmed)
        VALUES ((SELECT id from users WHERE tel = ?), ?, ?, ?, 3)`,
        rec.Endpoint,
        rec.FileName,
        rec.Created,
        rec.Created,
    ).Scan(&rec.ID)
}
