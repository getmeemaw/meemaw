// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.16.0

package database

import ()

type Device struct {
	ID        int64
	UserID    int64
	WalletID  int64
	PeerID    string
	UserAgent string
}

type User struct {
	ID         int64
	ForeignKey string
}

type Wallet struct {
	ID                  int64
	UserID              int64
	PublicAddress       string
	EncryptedDkgResults []byte
	Nonce               []byte
}
