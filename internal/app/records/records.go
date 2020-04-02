package records

import (
    "io"
    "os"
    "path"
    "strings"
    "time"

    "github.com/Gorynychdo/aster_go/internal/app/model"
    "github.com/Gorynychdo/aster_go/internal/app/store"
)

func Copy(dest, source, file string) error {
    s, err := os.Open(path.Join(source, file))
    if err != nil {
        return err
    }

    d, err := os.Create(path.Join(dest, file))
    if err != nil {
        return err
    }

    if _, err := io.Copy(d, s); err != nil {
        return err
    }

    return nil
}

func Persists(st *store.Store, endpoint, file string) error {
    if strings.HasPrefix(endpoint, "int_") {
        endpoint = strings.Replace(endpoint, "int_", "", 1)
    }

    rec := &model.Record{
        Endpoint: endpoint,
        FileName: path.Base(file),
        Created:  time.Now(),
    }

    return st.Record().Create(rec)
}
