package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type ClientRoles struct {
	Roles []string `json:"roles"`
}

type TokenInfo struct {
	Active         bool                   `json:"active"`
	Issuer         string                 `json:"iss"`
	ClientID       string                 `json:"client_id"`
	ResourceAccess map[string]ClientRoles `json:"resource_access"`
}

type AuthConfig struct {
	Endpoint         string
	Issuer           string
	ClientID         string
	ClientSecret     string
	InternalClientID string
	ProviderClients  map[string]string
}

func introspectToken(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	clientID string,
	clientSecret string,
	token string,
) (TokenInfo, error) {

	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("token", token)

	body := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return TokenInfo{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return TokenInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TokenInfo{}, fmt.Errorf("introspecção retornou HTTP %d", resp.StatusCode)
	}
	var info TokenInfo
	err = json.NewDecoder(resp.Body).Decode(&info)
	if err != nil {
		return TokenInfo{}, err
	}
	return info, nil
}

func requireWalletRole(
	client *http.Client,
	config AuthConfig,
	requiredRole string,
	next http.HandlerFunc,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "application/json")
		authorization := r.Header.Get("Authorization")
		parts := strings.Fields(authorization)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		info, err := introspectToken(r.Context(), client, config.Endpoint, config.ClientID, config.ClientSecret, parts[1])
		if err != nil {
			w.WriteHeader(503)
			json.NewEncoder(w).Encode(map[string]string{"error": "unavailable"})
			return
		}
		if info.Active == false || info.Issuer != config.Issuer {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		log.Printf(
			"auth: cliente=%q esperado=%q permissao=%q roles=%v",
			info.ClientID,
			config.InternalClientID,
			requiredRole,
			info.ResourceAccess[config.InternalClientID].Roles,
		)

		if info.ClientID != config.InternalClientID {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(403)
			json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
			return
		}
		roles := info.ResourceAccess[config.InternalClientID].Roles
		hasRole := false

		for _, role := range roles {
			if role == requiredRole {
				hasRole = true
				break
			}
		}
		if !hasRole {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(403)
			json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
			return
		}
		next(w, r)

	}
}

func requireProviderRole(
	client *http.Client,
	config AuthConfig,
	requiredRole string,
	next func(w http.ResponseWriter, r *http.Request, providerID string),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		authorization := r.Header.Get("Authorization")
		parts := strings.Fields(authorization)

		if len(parts) != 2 ||
			!strings.EqualFold(parts[0], "Bearer") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(
				map[string]string{"error": "unauthorized"},
			)
			return
		}

		info, err := introspectToken(
			r.Context(),
			client,
			config.Endpoint,
			config.ClientID,
			config.ClientSecret,
			parts[1],
		)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(
				map[string]string{"error": "auth_unavailable"},
			)
			return
		}

		if !info.Active || info.Issuer != config.Issuer {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(
				map[string]string{"error": "unauthorized"},
			)
			return
		}

		providerID, allowed := config.ProviderClients[info.ClientID]
		if !allowed || providerID == "" {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(
				map[string]string{"error": "forbidden"},
			)
			return
		}

		roles := info.ResourceAccess[info.ClientID].Roles
		hasRole := false

		for _, role := range roles {
			if role == requiredRole {
				hasRole = true
				break
			}
		}

		if !hasRole {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(
				map[string]string{"error": "forbidden"},
			)
			return
		}

		next(w, r, providerID)
	}
}
