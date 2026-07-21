package models

// ErrorResponse : API  error response data model
type ErrorResponse struct {
	Status int    `json:"status"`
	Err    string `json:"error"`
}

// DBErrorResponse : Database  error response data model
type DBErrorResponse struct {
	Type string `json:"type"`
	Code int    `json:"code"`
	Err  string `json:"error"`
}

// QueryNotFoundErrorResponse : API  error response data model
type QueryNotFoundErrorResponse struct {
	Status  int    `json:"status"`
	Err     string `json:"error"`
	Queries Query  `json:"query"`
}

// NotFoundErrorResponse : API  error response data model
type NotFoundErrorResponse struct {
	Status int    `json:"status"`
	Err    string `json:"error"`
	Query  string `json:"query"`
}

// SuccessResponse : API success response data model
type SuccessResponse struct {
	Status int   `json:"status"`
	Data   Quote `json:"data"`
}

// PostSuccessResponse : API POST success response data model
type PostSuccessResponse struct {
	Status int      `json:"status"`
	Data   NewQuote `json:"data"`
}

// DeleteResponse : API success response data model
type DeleteResponse struct {
	Status   int    `json:"status"`
	ID       string `json:"id"`
	Response string `json:"response"`
}

// TokenRequestSuccessResponse : Admin API sucessful post response
type TokenRequestSuccessResponse struct {
	Status int          `json:"status"`
	Data   TokenRequest `json:"data"`
}

// TarotNotFoundResponse : Tarot API  error response data model
type TarotNotFoundResponse struct {
	Status int    `json:"status"`
	Err    string `json:"error"`
}
