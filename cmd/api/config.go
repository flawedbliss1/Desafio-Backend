package main

import (
	"errors"
	"net/http"
	"os"
	"time"
)

func NewAuthConfig() (AuthConfig, error) {

	authConfig := AuthConfig{
		Endpoint:         "http://localhost:8081/realms/jungle/protocol/openid-connect/token/introspect",
		Issuer:           "http://localhost:8081/realms/jungle",
		ClientID:         "jungle-api",
		ClientSecret:     os.Getenv("JUNGLE_API_CLIENT_SECRET"),
		InternalClientID: "servico-interno",
		ProviderClients: map[string]string{
			"provider-a": "provider-a",
		},
	}

	if authConfig.ClientSecret == "" {
		return AuthConfig{}, errors.New("jungle api secret key vazia")
	}
	return authConfig, nil
}

func newHTTPClient() *http.Client {
	httpClient := &http.Client{
		Timeout: 3 * time.Second,
	}
	return httpClient
}
