package model

import "time"

type Record struct {
    ID       int
    Endpoint string
    FileName string
    Created  time.Time
}
