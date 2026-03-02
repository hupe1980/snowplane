package v1alpha1

// SecretShowOutput holds the output from SHOW SECRETS.
type SecretShowOutput struct {
	// +optional
	CreatedOn *string `json:"createdOn,omitempty"`
	// +optional
	Name *string `json:"name,omitempty"`
	// +optional
	DatabaseName *string `json:"databaseName,omitempty"`
	// +optional
	SchemaName *string `json:"schemaName,omitempty"`
	// +optional
	Owner *string `json:"owner,omitempty"`
	// +optional
	Comment *string `json:"comment,omitempty"`
	// +optional
	SecretType *string `json:"secretType,omitempty"`
	// +optional
	OAuthScopes *string `json:"oauthScopes,omitempty"`
}

// SecretDescribeOutput holds the output from DESCRIBE SECRET.
type SecretDescribeOutput struct {
	// +optional
	SecretType *string `json:"secretType,omitempty"`
	// +optional
	Username *string `json:"username,omitempty"`
	// +optional
	OAuthAccessTokenExpiryTime *string `json:"oauthAccessTokenExpiryTime,omitempty"`
	// +optional
	OAuthRefreshTokenExpiryTime *string `json:"oauthRefreshTokenExpiryTime,omitempty"`
	// +optional
	OAuthScopes *string `json:"oauthScopes,omitempty"`
	// +optional
	IntegrationName *string `json:"integrationName,omitempty"`
	// +optional
	Comment *string `json:"comment,omitempty"`
}
