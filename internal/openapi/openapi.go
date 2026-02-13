package openapi

func envelopeSchema(dataSchema map[string]any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code": map[string]any{"type": "integer"},
			"err":  map[string]any{"type": "string"},
			"data": dataSchema,
		},
		"required": []string{"code"},
	}
}

// Spec returns a minimal OpenAPI 3 spec for logtap HTTP API.
// It is intentionally hand-maintained to avoid codegen tooling.
func Spec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "logtap API",
			"version": "0.1.0",
		},
		"paths": map[string]any{
			"/healthz": map[string]any{
				"get": map[string]any{
					"tags":        []string{"system"},
					"summary":     "Health check",
					"responses":   map[string]any{"200": map[string]any{"description": "OK"}},
					"operationId": "healthz",
				},
			},
			"/api/status": map[string]any{
				"get": map[string]any{
					"tags":        []string{"system"},
					"summary":     "Get system status",
					"operationId": "getSystemStatus",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Status",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/SystemStatus"}),
								},
							},
						},
					},
				},
			},
			"/api/auth/bootstrap": map[string]any{
				"post": map[string]any{
					"tags":        []string{"auth"},
					"summary":     "Bootstrap first admin user and default project",
					"operationId": "bootstrap",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/BootstrapRequest"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Bootstrapped",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/BootstrapResponse"}),
								},
							},
						},
						"409": map[string]any{"description": "Already initialized"},
						"503": map[string]any{"description": "AUTH_SECRET not configured or database unavailable"},
					},
				},
			},
			"/api/auth/login": map[string]any{
				"post": map[string]any{
					"tags":        []string{"auth"},
					"summary":     "Login",
					"operationId": "login",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/LoginRequest"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Logged in",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/LoginResponse"}),
								},
							},
						},
						"401": map[string]any{"description": "Invalid credentials"},
						"503": map[string]any{"description": "AUTH_SECRET not configured or database unavailable"},
					},
				},
			},
			"/api/me": map[string]any{
				"get": map[string]any{
					"tags":        []string{"auth"},
					"summary":     "Get current user",
					"operationId": "getMe",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "User",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"user": map[string]any{"$ref": "#/components/schemas/User"},
										},
										"required": []string{"user"},
									}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
					},
				},
			},
			"/api/projects": map[string]any{
				"get": map[string]any{
					"tags":        []string{"projects"},
					"summary":     "List projects",
					"operationId": "listProjects",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Projects",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/Project"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
					},
				},
				"post": map[string]any{
					"tags":        []string{"projects"},
					"summary":     "Create project",
					"operationId": "createProject",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name": map[string]any{"type": "string"},
									},
									"required": []string{"name"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Project",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/Project"}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
					},
				},
			},
			"/api/projects/{projectId}": map[string]any{
				"get": map[string]any{
					"tags":        []string{"projects"},
					"summary":     "Get project",
					"operationId": "getProject",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Project",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/Project"}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"projects"},
					"summary":     "Delete project",
					"operationId": "deleteProject",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"deleted": map[string]any{"type": "boolean"},
										},
										"required": []string{"deleted"},
									}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
					},
				},
			},
			"/api/projects/{projectId}/keys": map[string]any{
				"get": map[string]any{
					"tags":        []string{"projects"},
					"summary":     "List project keys",
					"operationId": "listProjectKeys",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Keys",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/ProjectKey"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
					},
				},
				"post": map[string]any{
					"tags":        []string{"projects"},
					"summary":     "Create project key",
					"operationId": "createProjectKey",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name": map[string]any{"type": "string"},
									},
									"required": []string{"name"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Key",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/ProjectKey"}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
					},
				},
			},
			"/api/projects/{projectId}/keys/{keyId}/revoke": map[string]any{
				"post": map[string]any{
					"tags":        []string{"projects"},
					"summary":     "Revoke project key",
					"operationId": "revokeProjectKey",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "keyId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Revoked",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"revoked": map[string]any{"type": "boolean"},
										},
										"required": []string{"revoked"},
									}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
					},
				},
			},
			"/api/{projectId}/logs/": map[string]any{
				"post": map[string]any{
					"tags":        []string{"ingest"},
					"summary":     "Ingest custom logs (single or batch)",
					"operationId": "ingestLogs",
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":        "X-Project-Key",
							"in":          "header",
							"required":    false,
							"description": "Required when AUTH_SECRET is enabled (pk_...)",
							"schema":      map[string]any{"type": "string"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"oneOf": []any{
										map[string]any{"$ref": "#/components/schemas/CustomLogPayload"},
										map[string]any{
											"type":  "array",
											"items": map[string]any{"$ref": "#/components/schemas/CustomLogPayload"},
										},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"202": map[string]any{"description": "Accepted"},
						"400": map[string]any{"description": "Invalid payload"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Queue unavailable"},
					},
				},
			},
			"/api/{projectId}/logs/cleanup": map[string]any{
				"delete": map[string]any{
					"tags":        []string{"logs"},
					"summary":     "Cleanup logs before timestamp",
					"operationId": "cleanupLogs",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":        "before",
							"in":          "query",
							"required":    true,
							"description": "Delete logs with timestamp < before (RFC3339)",
							"schema":      map[string]any{"type": "string", "format": "date-time"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Cleanup result",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"deleted": map[string]any{"type": "integer"},
										},
										"required": []string{"deleted"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/events/cleanup": map[string]any{
				"delete": map[string]any{
					"tags":        []string{"events"},
					"summary":     "Cleanup events before timestamp",
					"operationId": "cleanupEvents",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":        "before",
							"in":          "query",
							"required":    true,
							"description": "Delete events with timestamp < before (RFC3339)",
							"schema":      map[string]any{"type": "string", "format": "date-time"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Cleanup result",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"deleted": map[string]any{"type": "integer"},
										},
										"required": []string{"deleted"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/track/": map[string]any{
				"post": map[string]any{
					"tags":        []string{"ingest"},
					"summary":     "Track events (single or batch)",
					"operationId": "trackEvents",
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":        "X-Project-Key",
							"in":          "header",
							"required":    false,
							"description": "Required when AUTH_SECRET is enabled (pk_...)",
							"schema":      map[string]any{"type": "string"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"oneOf": []any{
										map[string]any{"$ref": "#/components/schemas/TrackEventPayload"},
										map[string]any{
											"type":  "array",
											"items": map[string]any{"$ref": "#/components/schemas/TrackEventPayload"},
										},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"202": map[string]any{"description": "Accepted"},
						"400": map[string]any{"description": "Invalid payload"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Queue unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/contacts": map[string]any{
				"get": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "List alert contacts",
					"operationId": "listAlertContacts",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "type",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "string", "enum": []string{"email", "sms"}},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Contacts",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/AlertContact"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"post": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Create alert contact",
					"operationId": "createAlertContact",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"type":  map[string]any{"type": "string", "enum": []string{"email", "sms"}},
										"name":  map[string]any{"type": "string"},
										"value": map[string]any{"type": "string"},
									},
									"required": []string{"type", "value"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Contact",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertContact"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"409": map[string]any{"description": "Already exists"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/contacts/{contactId}": map[string]any{
				"put": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Update alert contact",
					"operationId": "updateAlertContact",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "contactId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name":  map[string]any{"type": "string"},
										"value": map[string]any{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Contact",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertContact"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"409": map[string]any{"description": "Already exists"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Delete alert contact",
					"operationId": "deleteAlertContact",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "contactId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"deleted": map[string]any{"type": "boolean"},
										},
										"required": []string{"deleted"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/contact-groups": map[string]any{
				"get": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "List alert contact groups",
					"operationId": "listAlertContactGroups",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "type",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "string", "enum": []string{"email", "sms"}},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Groups",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/AlertContactGroup"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"post": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Create alert contact group",
					"operationId": "createAlertContactGroup",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"type":             map[string]any{"type": "string", "enum": []string{"email", "sms"}},
										"name":             map[string]any{"type": "string"},
										"memberContactIds": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
									},
									"required": []string{"type", "name"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Group",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertContactGroup"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"409": map[string]any{"description": "Already exists"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/contact-groups/{groupId}": map[string]any{
				"put": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Update alert contact group",
					"operationId": "updateAlertContactGroup",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "groupId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name":             map[string]any{"type": "string"},
										"memberContactIds": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Group",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertContactGroup"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"409": map[string]any{"description": "Already exists"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Delete alert contact group",
					"operationId": "deleteAlertContactGroup",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "groupId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"deleted": map[string]any{"type": "boolean"},
										},
										"required": []string{"deleted"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/wecom-bots": map[string]any{
				"get": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "List WeCom bots",
					"operationId": "listAlertWecomBots",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Bots",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/AlertWecomBot"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"post": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Create WeCom bot",
					"operationId": "createAlertWecomBot",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name":       map[string]any{"type": "string"},
										"webhookUrl": map[string]any{"type": "string"},
									},
									"required": []string{"name", "webhookUrl"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Bot",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertWecomBot"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"409": map[string]any{"description": "Already exists"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/wecom-bots/{botId}": map[string]any{
				"put": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Update WeCom bot",
					"operationId": "updateAlertWecomBot",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "botId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name":       map[string]any{"type": "string"},
										"webhookUrl": map[string]any{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Bot",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertWecomBot"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"409": map[string]any{"description": "Already exists"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Delete WeCom bot",
					"operationId": "deleteAlertWecomBot",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "botId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"deleted": map[string]any{"type": "boolean"},
										},
										"required": []string{"deleted"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/webhook-endpoints": map[string]any{
				"get": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "List webhook endpoints",
					"operationId": "listAlertWebhookEndpoints",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Endpoints",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/AlertWebhookEndpoint"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"post": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Create webhook endpoint",
					"operationId": "createAlertWebhookEndpoint",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name": map[string]any{"type": "string"},
										"url":  map[string]any{"type": "string"},
									},
									"required": []string{"name", "url"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Endpoint",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertWebhookEndpoint"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"409": map[string]any{"description": "Already exists"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/webhook-endpoints/{endpointId}": map[string]any{
				"put": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Update webhook endpoint",
					"operationId": "updateAlertWebhookEndpoint",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "endpointId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name": map[string]any{"type": "string"},
										"url":  map[string]any{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Endpoint",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertWebhookEndpoint"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"409": map[string]any{"description": "Already exists"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Delete webhook endpoint",
					"operationId": "deleteAlertWebhookEndpoint",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "endpointId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"deleted": map[string]any{"type": "boolean"},
										},
										"required": []string{"deleted"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/rules": map[string]any{
				"get": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "List alert rules",
					"operationId": "listAlertRules",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Rules",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/AlertRule"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"post": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Create alert rule",
					"operationId": "createAlertRule",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name":    map[string]any{"type": "string"},
										"enabled": map[string]any{"type": "boolean"},
										"source":  map[string]any{"type": "string", "enum": []string{"logs", "events", "both"}},
										"match":   map[string]any{"type": "object", "additionalProperties": true},
										"repeat":  map[string]any{"type": "object", "additionalProperties": true},
										"targets": map[string]any{"type": "object", "additionalProperties": true},
									},
									"required": []string{"name", "source"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Rule",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertRule"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/rules/{ruleId}": map[string]any{
				"put": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Update alert rule",
					"operationId": "updateAlertRule",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "ruleId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"name":    map[string]any{"type": "string"},
										"enabled": map[string]any{"type": "boolean"},
										"source":  map[string]any{"type": "string", "enum": []string{"logs", "events", "both"}},
										"match":   map[string]any{"type": "object", "additionalProperties": true},
										"repeat":  map[string]any{"type": "object", "additionalProperties": true},
										"targets": map[string]any{"type": "object", "additionalProperties": true},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Rule",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{"$ref": "#/components/schemas/AlertRule"}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Delete alert rule",
					"operationId": "deleteAlertRule",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "ruleId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"deleted": map[string]any{"type": "boolean"},
										},
										"required": []string{"deleted"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"404": map[string]any{"description": "Not found"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/rules/test": map[string]any{
				"post": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "Test alert rules against an input (dry-run)",
					"operationId": "testAlertRules",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"source":  map[string]any{"type": "string", "enum": []string{"logs", "events", "both"}},
										"level":   map[string]any{"type": "string"},
										"message": map[string]any{"type": "string"},
										"fields":  map[string]any{"type": "object", "additionalProperties": true},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Preview",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/AlertRulePreview"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
			"/api/{projectId}/alerts/deliveries": map[string]any{
				"get": map[string]any{
					"tags":        []string{"alerts"},
					"summary":     "List alert deliveries (outbox)",
					"operationId": "listAlertDeliveries",
					"security":    []map[string]any{{"bearerAuth": []string{}}},
					"parameters": []map[string]any{
						{
							"name":     "projectId",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "status",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "string", "enum": []string{"pending", "processing", "sent", "failed"}},
						},
						{
							"name":     "channelType",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "string", "enum": []string{"wecom", "webhook", "email", "sms"}},
						},
						{
							"name":     "ruleId",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "limit",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Deliveries",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": envelopeSchema(map[string]any{
										"type": "object",
										"properties": map[string]any{
											"items": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/AlertDelivery"},
											},
										},
										"required": []string{"items"},
									}),
								},
							},
						},
						"400": map[string]any{"description": "Invalid request"},
						"401": map[string]any{"description": "Unauthorized"},
						"503": map[string]any{"description": "Database unavailable"},
					},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
			"schemas": map[string]any{
				"SystemStatus": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"status": map[string]any{
							"type": "string",
							"enum": []string{"uninitialized", "running", "maintenance", "exception"},
						},
						"initialized":  map[string]any{"type": "boolean"},
						"auth_enabled": map[string]any{"type": "boolean"},
						"message":      map[string]any{"type": "string"},
					},
					"required": []string{"status", "initialized", "auth_enabled"},
				},
				"User": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":    map[string]any{"type": "integer", "format": "int64"},
						"email": map[string]any{"type": "string"},
					},
					"required": []string{"id", "email"},
				},
				"LoginRequest": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"email":    map[string]any{"type": "string"},
						"password": map[string]any{"type": "string"},
					},
					"required": []string{"email", "password"},
				},
				"LoginResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"token": map[string]any{"type": "string"},
						"user":  map[string]any{"$ref": "#/components/schemas/User"},
					},
					"required": []string{"token", "user"},
				},
				"BootstrapRequest": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"email":        map[string]any{"type": "string"},
						"password":     map[string]any{"type": "string"},
						"project_name": map[string]any{"type": "string"},
						"key_name":     map[string]any{"type": "string"},
					},
					"required": []string{"email", "password"},
				},
				"BootstrapResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"token": map[string]any{"type": "string"},
						"user":  map[string]any{"$ref": "#/components/schemas/User"},
						"project": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id":   map[string]any{"type": "integer", "format": "int64"},
								"name": map[string]any{"type": "string"},
							},
							"required": []string{"id", "name"},
						},
						"key": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id":   map[string]any{"type": "integer", "format": "int64"},
								"name": map[string]any{"type": "string"},
								"key":  map[string]any{"type": "string"},
							},
							"required": []string{"id", "name", "key"},
						},
					},
					"required": []string{"token", "user", "project", "key"},
				},
				"Project": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":            map[string]any{"type": "integer", "format": "int64"},
						"owner_user_id": map[string]any{"type": "integer", "format": "int64"},
						"name":          map[string]any{"type": "string"},
					},
					"required": []string{"id", "owner_user_id", "name"},
				},
				"ProjectKey": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "integer", "format": "int64"},
						"project_id": map[string]any{"type": "integer", "format": "int64"},
						"name":       map[string]any{"type": "string"},
						"key":        map[string]any{"type": "string"},
						"created_at": map[string]any{"type": "string"},
						"revoked_at": map[string]any{"type": "string"},
					},
					"required": []string{"id", "project_id", "name", "key", "created_at"},
				},
				"CustomLogPayload": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"level": map[string]any{
							"type": "string",
							"enum": []string{"debug", "info", "warn", "error", "fatal", "event"},
						},
						"message":   map[string]any{"type": "string"},
						"device_id": map[string]any{"type": "string"},
						"trace_id":  map[string]any{"type": "string"},
						"span_id":   map[string]any{"type": "string"},
						"timestamp": map[string]any{"type": "string", "format": "date-time"},
						"fields": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
						"tags": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "string"},
						},
						"user": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
						"extra": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
						"sdk": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
						"contexts": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
					},
					"required": []string{"message"},
				},
				"TrackEventPayload": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":       map[string]any{"type": "string"},
						"device_id":  map[string]any{"type": "string"},
						"trace_id":   map[string]any{"type": "string"},
						"span_id":    map[string]any{"type": "string"},
						"timestamp":  map[string]any{"type": "string", "format": "date-time"},
						"properties": map[string]any{"type": "object", "additionalProperties": true},
						"tags": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "string"},
						},
						"user": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
						"extra": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
						"sdk": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
						"contexts": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						},
					},
					"required": []string{"name"},
				},
				"AlertContact": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "integer"},
						"project_id": map[string]any{"type": "integer"},
						"type":       map[string]any{"type": "string", "enum": []string{"email", "sms"}},
						"name":       map[string]any{"type": "string"},
						"value":      map[string]any{"type": "string"},
						"created_at": map[string]any{"type": "string", "format": "date-time"},
						"updated_at": map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "project_id", "type", "value", "created_at", "updated_at"},
				},
				"AlertContactGroup": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "integer"},
						"project_id": map[string]any{"type": "integer"},
						"type":       map[string]any{"type": "string", "enum": []string{"email", "sms"}},
						"name":       map[string]any{"type": "string"},
						"created_at": map[string]any{"type": "string", "format": "date-time"},
						"updated_at": map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "project_id", "type", "name", "created_at", "updated_at"},
				},
				"AlertWecomBot": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":          map[string]any{"type": "integer"},
						"project_id":  map[string]any{"type": "integer"},
						"name":        map[string]any{"type": "string"},
						"webhook_url": map[string]any{"type": "string"},
						"created_at":  map[string]any{"type": "string", "format": "date-time"},
						"updated_at":  map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "project_id", "name", "webhook_url", "created_at", "updated_at"},
				},
				"AlertWebhookEndpoint": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "integer"},
						"project_id": map[string]any{"type": "integer"},
						"name":       map[string]any{"type": "string"},
						"url":        map[string]any{"type": "string"},
						"created_at": map[string]any{"type": "string", "format": "date-time"},
						"updated_at": map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "project_id", "name", "url", "created_at", "updated_at"},
				},
				"AlertRule": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":         map[string]any{"type": "integer"},
						"project_id": map[string]any{"type": "integer"},
						"name":       map[string]any{"type": "string"},
						"enabled":    map[string]any{"type": "boolean"},
						"source":     map[string]any{"type": "string", "enum": []string{"logs", "events", "both"}},
						"match":      map[string]any{"type": "object", "additionalProperties": true},
						"repeat":     map[string]any{"type": "object", "additionalProperties": true},
						"targets":    map[string]any{"type": "object", "additionalProperties": true},
						"created_at": map[string]any{"type": "string", "format": "date-time"},
						"updated_at": map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "project_id", "name", "enabled", "source", "match", "repeat", "targets", "created_at", "updated_at"},
				},
				"AlertDelivery": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":              map[string]any{"type": "integer"},
						"project_id":      map[string]any{"type": "integer"},
						"rule_id":         map[string]any{"type": "integer"},
						"channel_type":    map[string]any{"type": "string", "enum": []string{"wecom", "webhook", "email", "sms"}},
						"target":          map[string]any{"type": "string"},
						"title":           map[string]any{"type": "string"},
						"content":         map[string]any{"type": "string"},
						"status":          map[string]any{"type": "string", "enum": []string{"pending", "processing", "sent", "failed"}},
						"attempts":        map[string]any{"type": "integer"},
						"next_attempt_at": map[string]any{"type": "string", "format": "date-time"},
						"last_error":      map[string]any{"type": "string"},
						"created_at":      map[string]any{"type": "string", "format": "date-time"},
						"updated_at":      map[string]any{"type": "string", "format": "date-time"},
					},
					"required": []string{"id", "project_id", "rule_id", "channel_type", "target", "title", "content", "status", "attempts", "next_attempt_at", "last_error", "created_at", "updated_at"},
				},
				"AlertDeliveryPreview": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"channelType": map[string]any{"type": "string"},
						"target":      map[string]any{"type": "string"},
						"title":       map[string]any{"type": "string"},
						"content":     map[string]any{"type": "string"},
					},
					"required": []string{"channelType", "target", "title", "content"},
				},
				"AlertRulePreview": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ruleId":              map[string]any{"type": "integer"},
						"ruleName":            map[string]any{"type": "string"},
						"matched":             map[string]any{"type": "boolean"},
						"dedupeKeyHash":       map[string]any{"type": "string"},
						"windowSec":           map[string]any{"type": "integer"},
						"threshold":           map[string]any{"type": "integer"},
						"occurrences":         map[string]any{"type": "integer"},
						"occurrencesBefore":   map[string]any{"type": "integer"},
						"occurrencesAfter":    map[string]any{"type": "integer"},
						"backoffExpBefore":    map[string]any{"type": "integer"},
						"backoffExpAfter":     map[string]any{"type": "integer"},
						"nextAllowedAtBefore": map[string]any{"type": "string", "format": "date-time"},
						"nextAllowedAtAfter":  map[string]any{"type": "string", "format": "date-time"},
						"windowExpired":       map[string]any{"type": "boolean"},
						"willEnqueue":         map[string]any{"type": "boolean"},
						"suppressedReason":    map[string]any{"type": "string"},
						"suppressedMessage":   map[string]any{"type": "string"},
						"deliveries": map[string]any{
							"type":  "array",
							"items": map[string]any{"$ref": "#/components/schemas/AlertDeliveryPreview"},
						},
					},
					"required": []string{"ruleId", "ruleName", "matched"},
				},
			},
		},
	}
}
