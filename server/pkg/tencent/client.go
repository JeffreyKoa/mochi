package tencent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	SecretID  string
	SecretKey string
	Region    string
	HTTP      *http.Client
}

func NewClient(secretID, secretKey, region string) *Client {
	if region == "" {
		region = "ap-guangzhou"
	}
	return &Client{
		SecretID:  secretID,
		SecretKey: secretKey,
		Region:    region,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, service, action, version string, payload any, result any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	host := service + ".tencentcloudapi.com"
	url := "https://" + host

	ts := time.Now().Unix()
	date := time.Unix(ts, 0).UTC().Format("2006-01-02")

	hashedPayload := sha256Hex(string(body))
	canonicalRequest := strings.Join([]string{
		"POST",
		"/",
		"",
		"content-type:application/json; charset=utf-8\nhost:" + host + "\n",
		"content-type;host",
		hashedPayload,
	}, "\n")

	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		fmt.Sprintf("%d", ts),
		credentialScope,
		sha256Hex(canonicalRequest),
	}, "\n")

	secretDate := hmacSHA256([]byte("TC3"+c.SecretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	auth := fmt.Sprintf(
		"TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=content-type;host, Signature=%s",
		c.SecretID, credentialScope, signature,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", ts))
	req.Header.Set("X-TC-Region", c.Region)
	req.Header.Set("Authorization", auth)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var envelope struct {
		Response json.RawMessage `json:"Response"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("tencent api decode: %s", string(respBody))
	}

	var errResp struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	}
	_ = json.Unmarshal(envelope.Response, &errResp)
	if errResp.Error != nil {
		return fmt.Errorf("tencent %s: %s", errResp.Error.Code, errResp.Error.Message)
	}

	if result != nil {
		if err := json.Unmarshal(envelope.Response, result); err != nil {
			return fmt.Errorf("tencent response: %w, body=%s", err, string(respBody))
		}
	}
	return nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}
