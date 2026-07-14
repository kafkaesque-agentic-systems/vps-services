package models

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Quote : Quote data model
type Quote struct {
	ID     primitive.ObjectID `json:"id" bson:"_id"`
	Author string             `json:"attribution" bson:"attribution"`
	Text   string             `json:"quote" bson:"quote"`
}

// NewQuote : Quote data model
type NewQuote struct {
	ID     primitive.ObjectID `json:"id" bson:"_id"`
	Author string             `json:"attribution" bson:"attribution"`
	Tag    string             `json:"-" bson:"ueid"`
	UID    string             `json:"-" bson:"uid"`
	Text   string             `json:"quote" bson:"quote"`
}

// Author : Author data model
type Author struct {
	Name   string  `json:"names" bson:"names"`
	Quotes []Quote `json:"quotes" bson:"quotes"`
}

// Authors : list of authors data model
type Authors struct {
	Names []interface{} `json:"names" bson:"names"`
}

// Query : used to unpack incoming search terms and phrases
type Query struct {
	Terms   []string `json:"terms" bson:"terms"`
	Phrases []string `json:"phrases" bson:"phrases"`
}
