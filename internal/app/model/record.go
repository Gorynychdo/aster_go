package model

import "time"

type Record struct {
    Endpoint string
    FileName string
    Created  time.Time
}
