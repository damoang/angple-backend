package ws

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/redis/go-redis/v9"
)

const redisPubSubChannel = "notifications"

// Event represents a real-time notification event sent via WebSocket
type Event struct {
	Type    string      `json:"type"`    // "notification", "unread_count"
	Payload interface{} `json:"payload"` // event-specific data
}

// Hub manages WebSocket clients and broadcasts messages
type Hub struct {
	// Registered clients grouped by member ID
	clients map[string]map[*Client]bool

	// Register/unregister channels
	register   chan *Client
	unregister chan *Client

	// Broadcast to a specific member
	broadcast chan *targetedEvent

	mu          sync.RWMutex
	redisClient *redis.Client
	ctx         context.Context
	cancel      context.CancelFunc
}

type targetedEvent struct {
	MemberID string
	Event    *Event
}

// NewHub creates a new Hub
func NewHub(redisClient *redis.Client) *Hub {
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{
		clients:     make(map[string]map[*Client]bool),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan *targetedEvent, 256),
		redisClient: redisClient,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Register adds a client to the hub
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	// Start Redis subscriber if Redis is available
	if h.redisClient != nil {
		go h.subscribeRedis()
	}

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.clients[client.memberID] == nil {
				h.clients[client.memberID] = make(map[*Client]bool)
			}
			h.clients[client.memberID][client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.memberID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.send)
					if len(clients) == 0 {
						delete(h.clients, client.memberID)
					}
				}
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			if clients, ok := h.clients[msg.MemberID]; ok {
				data, err := json.Marshal(msg.Event)
				if err == nil {
					for client := range clients {
						select {
						case client.send <- data:
						default:
							close(client.send)
							delete(clients, client)
						}
					}
				}
			}
			h.mu.RUnlock()

		case <-h.ctx.Done():
			return
		}
	}
}

// SendToMember sends an event to a specific member (local + Redis publish)
func (h *Hub) SendToMember(memberID string, event *Event) {
	// Local broadcast
	h.broadcast <- &targetedEvent{MemberID: memberID, Event: event}

	// Publish to Redis for multi-instance support
	if h.redisClient != nil {
		msg := &redisMessage{MemberID: memberID, Event: event}
		data, err := json.Marshal(msg)
		if err == nil {
			h.redisClient.Publish(h.ctx, redisPubSubChannel, data) //nolint:errcheck
		}
	}
}

type redisMessage struct {
	MemberID string `json:"member_id"`
	Event    *Event `json:"event"`
}

// subscribeRedis listens for notifications from other instances
func (h *Hub) subscribeRedis() {
	pubsub := h.redisClient.Subscribe(h.ctx, redisPubSubChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var rm redisMessage
			if err := json.Unmarshal([]byte(msg.Payload), &rm); err == nil {
				// Only local broadcast (don't re-publish to Redis)
				h.broadcast <- &targetedEvent{MemberID: rm.MemberID, Event: rm.Event}
			}
		case <-h.ctx.Done():
			return
		}
	}
}

// Stop gracefully shuts down the hub
func (h *Hub) Stop() {
	h.cancel()
}
