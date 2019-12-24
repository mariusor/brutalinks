package app

import "time"

type FollowRequest struct {
	Hash        Hash      `json:"hash"`
	SubmittedAt time.Time `json:"-"`
	SubmittedBy *Account  `json:"-"`
	Object      *Account  `json:"-"`
	Flags       FlagBits  `json:"-"`
}
