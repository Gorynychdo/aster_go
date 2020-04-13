package model

import "time"

type Record struct {
    Endpoint     string
    FileName     string
    Interlocutor string
    Created      time.Time
}
