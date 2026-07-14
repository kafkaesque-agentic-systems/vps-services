package models

import (
    "go.mongodb.org/mongo-driver/bson/primitive"
)

// SingleCard : used for unpacking card from random queries
type SingleCard struct {
    Card string `json:"card" bson:"cards"`
}

// Card : tarot card data model
type Card struct {
    ID   primitive.ObjectID `json:"id" bson:"_id"`
    Deck string             `json:"deck" bson:"name"`
    Card string             `json:"card"`
}

// Deck : tarot deck data model
type Deck struct {
    ID    primitive.ObjectID `json:"id" bson:"_id"`
    Name  string             `json:"name" bson:"name"`
    Cards []string           `json:"cards" bson:"cards"`
}

// Decks : model for a list of available tarot decks
type Decks struct {
    Names []interface{} `json:"names" bson:"names"`
}

// Reading : tarot reading data model
type Reading struct {
    Name   string            `json:"name"`
    Layout map[string]string `json:"spread"`
}

// CustomSpread : used to unpack custom tarot spread positions
type CustomSpread struct {
    Name      string   `json:"name" bson:"name"`
    Deck      string   `json:"deck" bson:"deck"`            // erotic, random, randall
    Positions []string `json:"positions" bson:"positions"`
}
