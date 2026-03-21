package main

import (
	"context"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// waClient is the global WhatsApp client singleton.
var waClient *WAClient

// WAClient wraps a whatsmeow client with pairing state management.
type WAClient struct {
	mu        sync.RWMutex
	client    *whatsmeow.Client
	container *sqlstore.Container

	// Pairing state
	pairMu  sync.Mutex
	qrCode  string        // latest QR code string
	paired  chan struct{} // closed when pairing completes
	pairing bool          // whether a pairing flow is in progress
}

// NewWAClient creates a new WhatsApp client backed by a SQLite device store.
func NewWAClient(dbPath string) (*WAClient, error) {
	container, err := sqlstore.New(context.Background(), "sqlite3",
		fmt.Sprintf("file:%s?_foreign_keys=on", dbPath),
		waLog.Stdout("whatsmeow-db", "WARN", true),
	)
	if err != nil {
		return nil, fmt.Errorf("whatsmeow sqlstore: %w", err)
	}

	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("whatsmeow device: %w", err)
	}

	client := whatsmeow.NewClient(device, waLog.Stdout("whatsmeow", "WARN", true))

	wac := &WAClient{
		client:    client,
		container: container,
	}

	client.AddEventHandler(wac.eventHandler)
	return wac, nil
}

// eventHandler handles whatsmeow events (logging only for now).
func (w *WAClient) eventHandler(evt interface{}) {
	switch evt.(type) {
	case *events.Disconnected:
		log.Println("whatsapp: disconnected")
	case *events.Connected:
		log.Println("whatsapp: connected")
	}
}

// Connect connects to WhatsApp using stored credentials.
// Returns an error if not yet paired (call StartPairing instead).
func (w *WAClient) Connect(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.client.Store.ID == nil {
		return fmt.Errorf("not paired — call StartPairing first")
	}
	return w.client.Connect()
}

// IsPaired returns true if the device has stored WhatsApp credentials.
func (w *WAClient) IsPaired() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.client.Store.ID != nil
}

// IsConnected returns true if the WhatsApp WebSocket is connected.
func (w *WAClient) IsConnected() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.client.IsConnected()
}

// PhoneNumber returns the paired phone number, or empty if not paired.
func (w *WAClient) PhoneNumber() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.client.Store.ID == nil {
		return ""
	}
	return "+" + w.client.Store.ID.User
}

// StartPairing initiates the QR code pairing flow.
// Returns the initial QR code string and a channel that closes when pairing completes.
func (w *WAClient) StartPairing(_ context.Context) (string, <-chan struct{}, error) {
	w.pairMu.Lock()
	defer w.pairMu.Unlock()

	if w.pairing {
		// Already pairing — return current state
		return w.qrCode, w.paired, nil
	}
	if w.IsPaired() {
		return "", nil, fmt.Errorf("already paired")
	}

	// Use a background context because the pairing flow must outlive the
	// HTTP request that initiated it (the user needs time to scan the QR).
	qrChan, err := w.client.GetQRChannel(context.Background())
	if err != nil {
		return "", nil, fmt.Errorf("get QR channel: %w", err)
	}

	if err := w.client.Connect(); err != nil {
		return "", nil, fmt.Errorf("connect: %w", err)
	}

	w.paired = make(chan struct{})
	w.pairing = true

	// Wait for first QR code
	firstQR := <-qrChan
	if firstQR.Event != "code" {
		w.pairing = false
		return "", nil, fmt.Errorf("unexpected QR event: %s", firstQR.Event)
	}
	w.qrCode = firstQR.Code

	// Background goroutine watches for QR rotations and pairing success
	go func() {
		for item := range qrChan {
			w.pairMu.Lock()
			switch item.Event {
			case "code":
				w.qrCode = item.Code
			default:
				// success, timeout, or error
				w.pairing = false
				w.qrCode = ""
				if item.Event == "success" {
					log.Println("whatsapp: pairing successful")
				} else {
					log.Printf("whatsapp: pairing ended: %s", item.Event)
				}
				close(w.paired)
				w.pairMu.Unlock()
				return
			}
			w.pairMu.Unlock()
		}
	}()

	return w.qrCode, w.paired, nil
}

// CurrentQR returns the latest QR code string and whether pairing is still in progress.
func (w *WAClient) CurrentQR() (string, bool) {
	w.pairMu.Lock()
	defer w.pairMu.Unlock()
	return w.qrCode, w.pairing
}

// SendDocument uploads and sends a file as a WhatsApp document message.
func (w *WAClient) SendDocument(ctx context.Context, to string, filePath string, caption string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if !w.client.IsConnected() {
		return fmt.Errorf("whatsapp not connected")
	}

	jid, err := phoneToJID(to)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	mimeType := detectMIME(filePath)

	uploadResp, err := w.client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	fileName := filepath.Base(filePath)
	msg := &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			URL:           proto.String(uploadResp.URL),
			DirectPath:    proto.String(uploadResp.DirectPath),
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    proto.Uint64(uploadResp.FileLength),
			Mimetype:      proto.String(mimeType),
			FileName:      proto.String(fileName),
			Title:         proto.String(fileName),
		},
	}
	if caption != "" {
		msg.DocumentMessage.Caption = proto.String(caption)
	}

	_, err = w.client.SendMessage(ctx, jid, msg)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

// SendText sends a plain text message to the given phone number.
func (w *WAClient) SendText(ctx context.Context, to string, text string) error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if !w.client.IsConnected() {
		return fmt.Errorf("whatsapp not connected")
	}

	jid, err := phoneToJID(to)
	if err != nil {
		return err
	}

	msg := &waE2E.Message{
		Conversation: proto.String(text),
	}
	_, err = w.client.SendMessage(ctx, jid, msg)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

// Disconnect disconnects the WhatsApp client.
func (w *WAClient) Disconnect() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.client.Disconnect()
}

// Logout unlinks the device from WhatsApp and clears stored credentials.
func (w *WAClient) Logout(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.client.Logout(ctx)
}

// phoneToJID converts a phone number string to a WhatsApp JID.
func phoneToJID(phone string) (types.JID, error) {
	cleaned := strings.TrimLeft(phone, "+")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	if cleaned == "" {
		return types.JID{}, fmt.Errorf("empty phone number")
	}
	return types.NewJID(cleaned, types.DefaultUserServer), nil
}

// detectMIME returns the MIME type for a file based on its extension.
func detectMIME(filePath string) string {
	ext := filepath.Ext(filePath)
	m := mime.TypeByExtension(ext)
	if m == "" {
		m = "application/octet-stream"
	}
	return m
}
