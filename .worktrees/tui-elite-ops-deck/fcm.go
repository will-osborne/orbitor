package main

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// FCMSender sends Firebase Cloud Messaging push notifications using the
// FCM v1 HTTP API with service account credentials. It caches the access
// token and refreshes it before expiry. All sends are best-effort.
type FCMSender struct {
	projectID   string
	clientEmail string
	privateKey  *rsa.PrivateKey

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time

	store      *Store
	summarizer *Summarizer // optional summarizer using local Ollama/llamafile
}

type serviceAccountJSON struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// NewFCMSender loads a service account JSON file and returns a ready sender.
// Returns nil (not an error) if path is empty, so callers can skip gracefully.
func NewFCMSender(serviceAccountPath string, store *Store, summarizer *Summarizer) *FCMSender {
	if serviceAccountPath == "" {
		return nil
	}
	data, err := os.ReadFile(serviceAccountPath)
	if err != nil {
		log.Printf("fcm: read service account: %v", err)
		return nil
	}
	var sa serviceAccountJSON
	if err := json.Unmarshal(data, &sa); err != nil {
		log.Printf("fcm: parse service account: %v", err)
		return nil
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		log.Printf("fcm: no PEM block in private key")
		return nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		log.Printf("fcm: parse private key: %v", err)
		return nil
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		log.Printf("fcm: private key is not RSA")
		return nil
	}
	log.Printf("fcm: loaded service account %s for project %s", sa.ClientEmail, sa.ProjectID)
	return &FCMSender{
		projectID:   sa.ProjectID,
		clientEmail: sa.ClientEmail,
		privateKey:  rsaKey,
		store:       store,
		summarizer:  summarizer,
	}
}

// Send pushes a notification to all registered device tokens. It is
// best-effort: errors are logged but not returned to the caller.
func (f *FCMSender) Send(title, body string, data map[string]string) {
	if f == nil {
		return
	}
	// Try to generate a short summary using the local summarizer (ollama/llamafile)
	summTitle := title
	summBody := body
	if f.summarizer != nil && f.store != nil {
		// If a sessionId is provided in data, load recent messages for context.
		if sid, ok := data["sessionId"]; ok && sid != "" {
			msgs, err := f.store.LoadRecentMessages(sid, 200)
			if err == nil && len(msgs) > 0 {
				if t, s := f.summarizer.Summarize(msgs); t != "" || s != "" {
					if t != "" {
						summTitle = t
					}
					if s != "" {
						summBody = s
					}
				}
			}
		} else {
			// No session context — attempt a light summarization from title+body
			ctxText := fmt.Sprintf("Title: %s\nBody: %s", title, body)
			if t, s := f.summarizer.SummarizeText(ctxText); t != "" || s != "" {
				if t != "" {
					summTitle = t
				}
				if s != "" {
					summBody = s
				}
			}
		}
	}

	// If the event indicates the agent is awaiting user input, make that clear.
	if data != nil {
		if et, ok := data["eventType"]; ok {
			switch et {
			case "permission_request":
				summBody = summBody + " — awaiting permission"
			case "prompt_needed", "user_input", "input_required":
				summBody = summBody + " — awaiting input"
			}
		}
	}

	tokens, err := f.store.ListDeviceTokens()
	if err != nil || len(tokens) == 0 {
		return
	}
	token, err := f.getAccessToken()
	if err != nil {
		log.Printf("fcm: get access token: %v", err)
		return
	}
	for _, deviceToken := range tokens {
		if err := f.sendToToken(token, deviceToken, summTitle, summBody, data); err != nil {
			log.Printf("fcm: send to %s…: %v", deviceToken[:min(8, len(deviceToken))], err)
			// Remove invalid tokens so they don't accumulate.
			if isInvalidToken(err) {
				_ = f.store.DeleteDeviceToken(deviceToken)
			}
		}
	}
}

func (f *FCMSender) sendToToken(accessToken, deviceToken, title, body string, data map[string]string) error {
	if data == nil {
		data = map[string]string{}
	}
	payload := map[string]any{
		"message": map[string]any{
			"token": deviceToken,
			"notification": map[string]string{
				"title": title,
				"body":  body,
			},
			"android": map[string]any{
				"priority": "high",
				"notification": map[string]string{
					"channel_id": "agent_events",
				},
			},
			"data": data,
		},
	}
	raw, _ := json.Marshal(payload)
	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", f.projectID)
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("FCM %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

func isInvalidToken(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "UNREGISTERED") || strings.Contains(s, "INVALID_ARGUMENT")
}

// getAccessToken returns a cached OAuth2 token or fetches a fresh one.
func (f *FCMSender) getAccessToken() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.accessToken != "" && time.Now().Before(f.tokenExpiry) {
		return f.accessToken, nil
	}
	token, expiry, err := f.fetchAccessToken()
	if err != nil {
		return "", err
	}
	f.accessToken = token
	f.tokenExpiry = expiry
	return token, nil
}

const googleTokenURI = "https://oauth2.googleapis.com/token"

func (f *FCMSender) fetchAccessToken() (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(55 * time.Minute)

	// Build JWT claims for service account auth.
	header := base64URLEncode(mustJSON(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}))
	claims := base64URLEncode(mustJSON(map[string]any{
		"iss":   f.clientEmail,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   googleTokenURI,
		"iat":   now.Unix(),
		"exp":   exp.Unix(),
	}))

	sigInput := header + "." + claims
	sig, err := signRS256(f.privateKey, []byte(sigInput))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign jwt: %w", err)
	}
	jwt := sigInput + "." + base64URLEncodeBytes(sig)

	resp, err := http.PostForm(googleTokenURI, url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {jwt},
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", time.Time{}, fmt.Errorf("token response %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", time.Time{}, fmt.Errorf("parse token: %w", err)
	}
	actualExpiry := now.Add(time.Duration(result.ExpiresIn-60) * time.Second)
	return result.AccessToken, actualExpiry, nil
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func base64URLEncodeBytes(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func signRS256(key *rsa.PrivateKey, data []byte) ([]byte, error) {
	h := sha256.New()
	h.Write(data)
	digest := h.Sum(nil)
	return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest)
}
