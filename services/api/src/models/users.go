package models

// User : API user
type User struct {
	UID   string `json:"uid" bson:"uid"`
	Email string `json:"email" bson:"email"`
	// O-4: the previous tag was `json:"authorization" json:"authorization"` —
	// a duplicate json key with NO bson tag, so the secret failed to map from
	// Mongo (field "authorization") AND would serialize outward in JSON.
	// It is now correctly mapped from bson and never emitted in JSON responses.
	Authorization string `json:"-" bson:"authorization"`
}
