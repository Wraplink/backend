package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (h *GoogleAuthHandler) fetchGoogleProfile(ctx context.Context, code, verifier string) (googleProfile, error) {
	var profile googleProfile
	body := url.Values{
		"code": {code}, "code_verifier": {verifier}, "grant_type": {"authorization_code"},
		"client_id": {h.clientID}, "client_secret": {h.clientSecret}, "redirect_uri": {h.redirectURI},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(body.Encode()))
	if err != nil {
		return profile, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var credentials struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := h.readGoogleResponse(request, &credentials); err != nil {
		return profile, err
	}
	if credentials.AccessToken == "" || !strings.EqualFold(credentials.TokenType, "Bearer") {
		return profile, errors.New("Google response did not contain a bearer token")
	}

	// Resolve the subject through Google's UserInfo endpoint using the access
	// token obtained server-side with this client's code and PKCE verifier.
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return profile, err
	}
	request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
	err = h.readGoogleResponse(request, &profile)
	return profile, err
}

func (h *GoogleAuthHandler) readGoogleResponse(request *http.Request, target any) error {
	response, err := h.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("Google rejected the OAuth request")
	}
	return json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(target)
}
