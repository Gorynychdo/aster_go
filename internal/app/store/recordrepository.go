package store

import "github.com/Gorynychdo/aster_go/internal/app/model"

type RecordRepository struct {
	store *Store
}

func (r *RecordRepository) Create(rec *model.Record) error {
    _, err:= r.store.db.Query(
        `INSERT INTO records
            (account_id, wav_file_name, creation_time, complete_time)
        VALUES ((SELECT id from users WHERE tel = ?), ?, ?, ?)`,
        rec.Endpoint,
        rec.FileName,
        rec.Created,
        rec.Created,
    )

    return err
}
