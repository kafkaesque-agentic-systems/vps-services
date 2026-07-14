package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// TokenRequest : Token Request Data
type TokenRequest struct {
	ID      primitive.ObjectID `json:"id" bson:"_id"`
	Email   string             `json:"email" bson:"email"`
	Granted string             `json:"granted" json:"granted"`
}

// TokenQueryResponse : Token Response Data
type TokenQueryResponse struct {
       Result int
}
