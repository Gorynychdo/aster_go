package records

import (
    "io"
    "os"
    "path"
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


