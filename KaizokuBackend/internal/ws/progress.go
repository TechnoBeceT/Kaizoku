package ws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

const (
	recordSeparator = '\x1e'
	writeWait       = 10 * time.Second
	pongWait        = 60 * time.Second
	pingPeriod      = 25 * time.Second
)

// SignalR message types
const (
	signalRInvocation     = 1
	signalRStreamItem     = 2
	signalRCompletion     = 3
	signalRStreamInvocat  = 4
	signalRCancelInvocat  = 5
	signalRPing           = 6
	signalRClose          = 7
)

// ProgressState is the message format sent to clients.
type ProgressState struct {
	ID             string      `json:"id"`
	JobType        int         `json:"jobType"`
	ProgressStatus int         `json:"progressStatus"`
	Percentage     float64     `json:"percentage"`
	Message        string      `json:"message"`
	ErrorMessage   string      `json:"errorMessage,omitempty"`
	Parameter      interface{} `json:"parameter,omitempty"`
}

// signalRMessage is a generic SignalR JSON message.
type signalRMessage struct {
	Type      int             `json:"type"`
	Target    string          `json:"target,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// negotiateResponse is the response to the /negotiate endpoint.
type negotiateResponse struct {
	ConnectionID        string      `json:"connectionId"`
	ConnectionToken     string      `json:"connectionToken"`
	NegotiateVersion    int         `json:"negotiateVersion"`
	AvailableTransports []transport `json:"availableTransports"`
}

type transport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Hub manages WebSocket connections and broadcasts progress updates.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]*client
}

type client struct {
	conn *websocket.Conn
	send chan []byte
}

// NewHub creates a new progress hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*client),
	}
}

// Broadcast sends a ProgressState to all connected clients.
func (h *Hub) Broadcast(state ProgressState) {
	args, err := json.Marshal([]interface{}{state})
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal progress state")
		return
	}

	msg := signalRMessage{
		Type:      signalRInvocation,
		Target:    "Progress",
		Arguments: args,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal signalr message")
		return
	}

	// Append record separator
	data = append(data, byte(recordSeparator))

	h.mu.RLock()
	defer h.mu.RUnlock()

	for id, c := range h.clients {
		select {
		case c.send <- data:
		default:
			log.Warn().Str("clientId", id).Msg("client send buffer full, dropping message")
		}
	}
}

// BroadcastProgress implements the ProgressBroadcaster interface from the job package.
func (h *Hub) BroadcastProgress(id string, jobType int, status int, percentage float64, message string, param interface{}) {
	h.Broadcast(ProgressState{
		ID:             id,
		JobType:        jobType,
		ProgressStatus: status,
		Percentage:     percentage,
		Message:        message,
		Parameter:      param,
	})
}

// HandleNegotiate handles the SignalR negotiate POST request.
func (h *Hub) HandleNegotiate(c echo.Context) error {
	connID := uuid.New().String()
	resp := negotiateResponse{
		ConnectionID:     connID,
		ConnectionToken:  connID,
		NegotiateVersion: 1,
		AvailableTransports: []transport{
			{
				Transport:       "WebSockets",
				TransferFormats: []string{"Text"},
			},
		},
	}
	return c.JSON(http.StatusOK, resp)
}

// HandleWebSocket handles the WebSocket upgrade and message loop.
func (h *Hub) HandleWebSocket(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}

	clientID := c.QueryParam("id")
	if clientID == "" {
		clientID = uuid.New().String()
	}

	cl := &client{
		conn: conn,
		send: make(chan []byte, 256),
	}

	h.mu.Lock()
	h.clients[clientID] = cl
	h.mu.Unlock()

	log.Debug().Str("clientId", clientID).Msg("progress client connected")

	// Handle SignalR handshake
	if err := h.handleHandshake(conn); err != nil {
		log.Warn().Err(err).Msg("SignalR handshake failed")
		h.removeClient(clientID)
		conn.Close()
		return nil
	}

	// Start writer goroutine
	go h.writePump(clientID, cl)

	// Reader loop (handles pings and close)
	h.readPump(clientID, cl)

	return nil
}

func (h *Hub) handleHandshake(conn *websocket.Conn) error {
	// Read handshake message from client
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return err
	}

	// Parse handshake - remove record separator
	if len(msg) > 0 && msg[len(msg)-1] == byte(recordSeparator) {
		msg = msg[:len(msg)-1]
	}

	// We don't need to validate the handshake strictly - just accept JSON protocol
	var handshake struct {
		Protocol string `json:"protocol"`
		Version  int    `json:"version"`
	}
	if err := json.Unmarshal(msg, &handshake); err != nil {
		return err
	}

	// Send handshake response (empty object = success)
	response := []byte("{}" + string(recordSeparator))
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	return conn.WriteMessage(websocket.TextMessage, response)
}

func (h *Hub) readPump(clientID string, cl *client) {
	defer func() {
		h.removeClient(clientID)
		cl.conn.Close()
	}()

	cl.conn.SetReadDeadline(time.Now().Add(pongWait))
	cl.conn.SetPongHandler(func(string) error {
		cl.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, msg, err := cl.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Debug().Err(err).Str("clientId", clientID).Msg("websocket read error")
			}
			return
		}

		// Parse SignalR messages (separated by \x1e)
		for _, part := range splitSignalR(msg) {
			if len(part) == 0 {
				continue
			}
			var m signalRMessage
			if err := json.Unmarshal(part, &m); err != nil {
				continue
			}

			switch m.Type {
			case signalRPing:
				// Respond with ping
				pong := []byte(`{"type":6}` + string(recordSeparator))
				select {
				case cl.send <- pong:
				default:
				}
			case signalRClose:
				return
			}
		}
	}
}

func (h *Hub) writePump(clientID string, cl *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		cl.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-cl.send:
			if !ok {
				cl.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cl.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cl.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Hub) removeClient(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if cl, ok := h.clients[clientID]; ok {
		close(cl.send)
		delete(h.clients, clientID)
		log.Debug().Str("clientId", clientID).Msg("progress client disconnected")
	}
}

func splitSignalR(data []byte) [][]byte {
	var parts [][]byte
	start := 0
	for i, b := range data {
		if b == byte(recordSeparator) {
			parts = append(parts, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		parts = append(parts, data[start:])
	}
	return parts
}
