package main

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub fans out messages to all connected WebSocket clients.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan WSMessage
	register   chan *Client
	unregister chan *Client
	disconnect chan struct{} // signal to close all clients (used during upgrades)
	history    []WSMessage
	historyMu  sync.RWMutex
	maxHistory int
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan WSMessage, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		disconnect: make(chan struct{}),
		maxHistory: 500,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.disconnect:
			// Close all client connections (used during graceful upgrades).
			// Don't return — keep the loop running so pending unregister
			// sends from ReadPump defers don't block.
			for client := range h.clients {
				client.conn.WriteMessage(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "server upgrading"),
				)
				close(client.send)
				delete(h.clients, client)
			}

		case client := <-h.register:
			h.clients[client] = true
			h.sendHistory(client)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case msg := <-h.broadcast:
			h.appendHistory(msg)
			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("hub: marshal error: %v", err)
				continue
			}
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (h *Hub) sendHistory(c *Client) {
	h.historyMu.RLock()
	defer h.historyMu.RUnlock()
	if len(h.history) == 0 {
		return
	}
	msgs, _ := json.Marshal(h.history)
	envelope := WSHistoryMessage{
		Type:     "history",
		Messages: h.history,
	}
	_ = msgs
	data, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

func (h *Hub) appendHistory(msg WSMessage) {
	h.historyMu.Lock()
	defer h.historyMu.Unlock()
	h.history = append(h.history, msg)
	if len(h.history) > h.maxHistory {
		h.history = h.history[len(h.history)-h.maxHistory:]
	}
}

// DisconnectClients sends a WebSocket close frame to every connected client
// and stops the Run loop. Used during graceful upgrades so clients get a clean
// close and trigger their auto-reconnect logic immediately.
func (h *Hub) DisconnectClients() {
	select {
	case h.disconnect <- struct{}{}:
	default:
	}
}

// SeedHistory directly populates the hub's history buffer with pre-loaded messages.
// Call this before starting Run() to avoid race conditions during startup.
func (h *Hub) SeedHistory(msgs []WSMessage) {
	if len(msgs) > h.maxHistory {
		msgs = msgs[len(msgs)-h.maxHistory:]
	}
	h.history = make([]WSMessage, len(msgs))
	copy(h.history, msgs)
}

func (h *Hub) Broadcast(msgType string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		log.Printf("hub: broadcast marshal error: %v", err)
		return
	}
	h.broadcast <- WSMessage{Type: msgType, Data: raw}
}

func (c *Client) WritePump() {
	defer c.conn.Close()
	for data := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

func (c *Client) ReadPump(onMessage func([]byte)) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if onMessage != nil {
			onMessage(msg)
		}
	}
}
