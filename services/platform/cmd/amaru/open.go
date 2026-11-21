package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/discovery"
)

func openInstance(ctx context.Context, open func(context.Context, string) error) error {
	dir, err := discovery.StateDirectory()
	if err != nil {
		return errors.New("local Instance discovery is unavailable")
	}
	info, err := discovery.Read(dir)
	if err != nil || !discovery.Live(ctx, info) {
		return errors.New("no reachable local Instance was found")
	}
	launch, err := requestLaunchURL(ctx, info)
	if err != nil {
		return err
	}
	if err := open(ctx, launch); err != nil {
		return errors.New("could not open the default browser")
	}
	return nil
}

// The caller supplies only a validated, live discovery record.
func requestLaunchURL(ctx context.Context, info discovery.Info) (string, error) {
	failed := errors.New("local Instance did not issue a usable browser launch code")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, info.Endpoint+"/v1/auth/launch-codes", nil)
	if err != nil {
		return "", failed
	}
	req.Header.Set("Authorization", "Bearer "+info.AdministratorSecret)
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", failed
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", failed
	}
	const maxResponse = 4096
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return "", failed
	}
	var result struct {
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return "", failed
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return "", failed
	}
	token, err := base64.RawURLEncoding.Strict().DecodeString(result.Code)
	if err != nil || len(token) != 32 || len(result.Code) != base64.RawURLEncoding.EncodedLen(32) || !result.ExpiresAt.After(time.Now()) {
		return "", failed
	}
	return info.Endpoint + "/auth/launch?" + url.Values{"code": {result.Code}}.Encode(), nil
}
