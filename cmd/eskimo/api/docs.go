// Code generated by swaggo/swag. DO NOT EDIT.

package api

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {
            "name": "ice.io",
            "url": "https://ice.io"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/user-statistics/top-countries": {
            "get": {
                "description": "Returns the paginated view of users per country.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Statistics"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "default": "Bearer \u003cAdd access token here\u003e",
                        "description": "Insert your access token",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "a keyword to look for in all country codes or names",
                        "name": "keyword",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "description": "Limit of elements to return. Defaults to 10",
                        "name": "limit",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "description": "Number of elements to skip before collecting elements to return",
                        "name": "offset",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/users.CountryStatistics"
                            }
                        }
                    },
                    "400": {
                        "description": "if validations fail",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "if not authorized",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "if syntax fails",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "504": {
                        "description": "if request times out",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/user-statistics/user-growth": {
            "get": {
                "description": "Returns statistics about user growth.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Statistics"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "default": "Bearer \u003cAdd access token here\u003e",
                        "description": "Insert your access token",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "integer",
                        "description": "number of days in the past to look for. Defaults to 3. Max is 90.",
                        "name": "days",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/users.UserGrowthStatistics"
                        }
                    },
                    "400": {
                        "description": "if validations fail",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "if not authorized",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "if syntax fails",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "504": {
                        "description": "if request times out",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/user-views/username": {
            "get": {
                "description": "Returns public information about an user account based on an username, making sure the username is valid first.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Accounts"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "default": "Bearer \u003cAdd access token here\u003e",
                        "description": "Insert your access token",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "username of the user. It will validate it first",
                        "name": "username",
                        "in": "query",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/users.UserProfile"
                        }
                    },
                    "400": {
                        "description": "if validations fail",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "if not authorized",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "404": {
                        "description": "if not found",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "if syntax fails",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "504": {
                        "description": "if request times out",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/users": {
            "get": {
                "description": "Returns a list of user account based on the provided query parameters.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Accounts"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "default": "Bearer \u003cAdd access token here\u003e",
                        "description": "Insert your access token",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "A keyword to look for in the usernames and full names of users",
                        "name": "keyword",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "integer",
                        "description": "Limit of elements to return. Defaults to 10",
                        "name": "limit",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "description": "Elements to skip before starting to look for",
                        "name": "offset",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/users.MinimalUserProfile"
                            }
                        }
                    },
                    "400": {
                        "description": "if validations fail",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "if not authorized",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "if syntax fails",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "504": {
                        "description": "if request times out",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/users/{userId}": {
            "get": {
                "description": "Returns an user's account.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Accounts"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "default": "Bearer \u003cAdd access token here\u003e",
                        "description": "Insert your access token",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "ID of the user",
                        "name": "userId",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/main.User"
                        }
                    },
                    "400": {
                        "description": "if validations fail",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "if not authorized",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "404": {
                        "description": "if not found",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "if syntax fails",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "504": {
                        "description": "if request times out",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/users/{userId}/referral-acquisition-history": {
            "get": {
                "description": "Returns the history of referral acquisition for the provided user id.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Referrals"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "default": "Bearer \u003cAdd access token here\u003e",
                        "description": "Insert your access token",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "ID of the user",
                        "name": "userId",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "integer",
                        "description": "Always is 5, cannot be changed due to DB schema",
                        "name": "days",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/users.ReferralAcquisition"
                            }
                        }
                    },
                    "400": {
                        "description": "if validations fail",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "if not authorized",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "if not allowed",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "if syntax fails",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "504": {
                        "description": "if request times out",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/users/{userId}/referrals": {
            "get": {
                "description": "Returns the referrals of an user.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Referrals"
                ],
                "parameters": [
                    {
                        "type": "string",
                        "default": "Bearer \u003cAdd access token here\u003e",
                        "description": "Insert your access token",
                        "name": "Authorization",
                        "in": "header",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "ID of the user",
                        "name": "userId",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "string",
                        "description": "Type of referrals: ` + "`" + `CONTACTS` + "`" + ` or ` + "`" + `T1` + "`" + ` or ` + "`" + `T2` + "`" + `",
                        "name": "type",
                        "in": "query",
                        "required": true
                    },
                    {
                        "type": "integer",
                        "description": "Limit of elements to return. Defaults to 10",
                        "name": "limit",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "description": "Number of elements to skip before collecting elements to return",
                        "name": "offset",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/users.Referrals"
                        }
                    },
                    "400": {
                        "description": "if validations fail",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "401": {
                        "description": "if not authorized",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "403": {
                        "description": "if not allowed",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "422": {
                        "description": "if syntax fails",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    },
                    "504": {
                        "description": "if request times out",
                        "schema": {
                            "$ref": "#/definitions/server.ErrorResponse"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "main.User": {
            "type": "object",
            "properties": {
                "agendaPhoneNumberHashes": {
                    "type": "string",
                    "example": "Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2"
                },
                "blockchainAccountAddress": {
                    "type": "string",
                    "example": "0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "checksum": {
                    "type": "string",
                    "example": "1232412415326543647657"
                },
                "city": {
                    "type": "string",
                    "example": "New York"
                },
                "clientData": {
                    "$ref": "#/definitions/users.JSON"
                },
                "country": {
                    "type": "string",
                    "example": "US"
                },
                "createdAt": {
                    "type": "string",
                    "example": "2022-01-03T16:20:52.156534Z"
                },
                "email": {
                    "type": "string",
                    "example": "jdoe@gmail.com"
                },
                "firstName": {
                    "type": "string",
                    "example": "John"
                },
                "hiddenProfileElements": {
                    "type": "array",
                    "items": {
                        "type": "string",
                        "enum": [
                            "globalRank",
                            "referralCount",
                            "level",
                            "role",
                            "badges"
                        ]
                    },
                    "example": [
                        "level"
                    ]
                },
                "id": {
                    "type": "string",
                    "example": "did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "language": {
                    "type": "string",
                    "example": "en"
                },
                "lastName": {
                    "type": "string",
                    "example": "Doe"
                },
                "miningBlockchainAccountAddress": {
                    "type": "string",
                    "example": "0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "phoneNumber": {
                    "type": "string",
                    "example": "+12099216581"
                },
                "profilePictureUrl": {
                    "type": "string",
                    "example": "https://somecdn.com/p1.jpg"
                },
                "referredBy": {
                    "type": "string",
                    "example": "did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "t1ReferralCount": {
                    "type": "integer",
                    "example": 100
                },
                "t2ReferralCount": {
                    "type": "integer",
                    "example": 100
                },
                "updatedAt": {
                    "type": "string",
                    "example": "2022-01-03T16:20:52.156534Z"
                },
                "username": {
                    "type": "string",
                    "example": "jdoe"
                }
            }
        },
        "server.ErrorResponse": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string",
                    "example": "SOMETHING_NOT_FOUND"
                },
                "data": {
                    "type": "object",
                    "additionalProperties": {}
                },
                "error": {
                    "type": "string",
                    "example": "something is missing"
                }
            }
        },
        "users.CountryStatistics": {
            "type": "object",
            "properties": {
                "country": {
                    "description": "ISO 3166 country code.",
                    "type": "string",
                    "example": "US"
                },
                "userCount": {
                    "type": "integer",
                    "example": 12121212
                }
            }
        },
        "users.JSON": {
            "type": "object",
            "additionalProperties": {}
        },
        "users.MinimalUserProfile": {
            "type": "object",
            "properties": {
                "active": {
                    "type": "boolean",
                    "example": true
                },
                "city": {
                    "type": "string",
                    "example": "New York"
                },
                "country": {
                    "type": "string",
                    "example": "US"
                },
                "email": {
                    "type": "string",
                    "example": "jdoe@gmail.com"
                },
                "id": {
                    "type": "string",
                    "example": "did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "phoneNumber": {
                    "type": "string",
                    "example": "+12099216581"
                },
                "pinged": {
                    "type": "boolean",
                    "example": false
                },
                "profilePictureUrl": {
                    "type": "string",
                    "example": "https://somecdn.com/p1.jpg"
                },
                "referralType": {
                    "enum": [
                        "CONTACTS",
                        "T0",
                        "T1",
                        "T2"
                    ],
                    "allOf": [
                        {
                            "$ref": "#/definitions/users.ReferralType"
                        }
                    ],
                    "example": "T1"
                },
                "username": {
                    "type": "string",
                    "example": "jdoe"
                }
            }
        },
        "users.ReferralAcquisition": {
            "type": "object",
            "properties": {
                "date": {
                    "type": "string",
                    "example": "2022-01-03"
                },
                "t1": {
                    "type": "integer",
                    "example": 22
                },
                "t2": {
                    "type": "integer",
                    "example": 13
                }
            }
        },
        "users.ReferralType": {
            "type": "string",
            "enum": [
                "CONTACTS",
                "T1",
                "T2"
            ],
            "x-enum-varnames": [
                "ContactsReferrals",
                "Tier1Referrals",
                "Tier2Referrals"
            ]
        },
        "users.Referrals": {
            "type": "object",
            "properties": {
                "active": {
                    "type": "integer",
                    "example": 11
                },
                "referrals": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/users.MinimalUserProfile"
                    }
                },
                "total": {
                    "type": "integer",
                    "example": 11
                }
            }
        },
        "users.UserCountTimeSeriesDataPoint": {
            "type": "object",
            "properties": {
                "active": {
                    "type": "integer",
                    "example": 11
                },
                "date": {
                    "type": "string",
                    "example": "2022-01-03T16:20:52.156534Z"
                },
                "total": {
                    "type": "integer",
                    "example": 11
                }
            }
        },
        "users.UserGrowthStatistics": {
            "type": "object",
            "properties": {
                "active": {
                    "type": "integer",
                    "example": 11
                },
                "timeSeries": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/users.UserCountTimeSeriesDataPoint"
                    }
                },
                "total": {
                    "type": "integer",
                    "example": 11
                }
            }
        },
        "users.UserProfile": {
            "type": "object",
            "properties": {
                "agendaPhoneNumberHashes": {
                    "type": "string",
                    "example": "Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2,Ef86A6021afCDe5673511376B2"
                },
                "blockchainAccountAddress": {
                    "type": "string",
                    "example": "0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "city": {
                    "type": "string",
                    "example": "New York"
                },
                "clientData": {
                    "$ref": "#/definitions/users.JSON"
                },
                "country": {
                    "type": "string",
                    "example": "US"
                },
                "createdAt": {
                    "type": "string",
                    "example": "2022-01-03T16:20:52.156534Z"
                },
                "email": {
                    "type": "string",
                    "example": "jdoe@gmail.com"
                },
                "firstName": {
                    "type": "string",
                    "example": "John"
                },
                "hiddenProfileElements": {
                    "type": "array",
                    "items": {
                        "type": "string",
                        "enum": [
                            "globalRank",
                            "referralCount",
                            "level",
                            "role",
                            "badges"
                        ]
                    },
                    "example": [
                        "level"
                    ]
                },
                "id": {
                    "type": "string",
                    "example": "did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "language": {
                    "type": "string",
                    "example": "en"
                },
                "lastName": {
                    "type": "string",
                    "example": "Doe"
                },
                "miningBlockchainAccountAddress": {
                    "type": "string",
                    "example": "0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "phoneNumber": {
                    "type": "string",
                    "example": "+12099216581"
                },
                "profilePictureUrl": {
                    "type": "string",
                    "example": "https://somecdn.com/p1.jpg"
                },
                "referredBy": {
                    "type": "string",
                    "example": "did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"
                },
                "t1ReferralCount": {
                    "type": "integer",
                    "example": 100
                },
                "t2ReferralCount": {
                    "type": "integer",
                    "example": 100
                },
                "updatedAt": {
                    "type": "string",
                    "example": "2022-01-03T16:20:52.156534Z"
                },
                "username": {
                    "type": "string",
                    "example": "jdoe"
                }
            }
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "latest",
	Host:             "",
	BasePath:         "/v1r",
	Schemes:          []string{"https"},
	Title:            "User Accounts, User Devices, User Statistics API",
	Description:      "API that handles everything related to read only operations for user's account, user's devices and statistics about accounts and devices.",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
