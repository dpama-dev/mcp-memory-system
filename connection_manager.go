package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

const (
	// Named pipe path for inter-process communication
	pipePath = "/tmp/mcp-memory-server.pipe"
	// Connection timeout
	connectionTimeout = 5 * time.Second
)

// ConnectionManager handles multi-client connections and server discovery
type ConnectionManager struct {
	isServer   bool
	listener   net.Listener
	clients    map[string]*ClientConnection
	clientsMu  sync.RWMutex
	store      *MemoryStore
	server     *MCPServer
}

// ClientConnection represents a connected MCP client
type ClientConnection struct {
	ID       string
	Conn     net.Conn
	Reader   *bufio.Reader
	Writer   *bufio.Writer
	LastSeen time.Time
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(store *MemoryStore, server *MCPServer) *ConnectionManager {
	return &ConnectionManager{
		clients: make(map[string]*ClientConnection),
		store:   store,
		server:  server,
	}
}

// Start attempts to connect to existing server or starts a new one
func (cm *ConnectionManager) Start() error {
	// Try to connect to existing server first
	conn, err := net.DialTimeout("unix", pipePath, connectionTimeout)
	if err == nil {
		// Server exists, run as client
		log.Println("Connecting to existing memory server...")
		return cm.runAsClient(conn)
	}

	// No existing server, start as server
	log.Println("Starting new memory server instance...")
	return cm.runAsServer()
}

// runAsClient forwards stdio to existing server
func (cm *ConnectionManager) runAsClient(conn net.Conn) error {
	defer conn.Close()
	
	// Create handoff request
	handoffMsg := MCPMessage{
		Jsonrpc: "2.0",
		Method:  "handoff/request",
		Params:  json.RawMessage(`{"client_id": "stdio"}`),
	}
	
	// Send handoff request
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(handoffMsg); err != nil {
		return fmt.Errorf("failed to send handoff request: %w", err)
	}
	
	// Set up bidirectional forwarding
	errChan := make(chan error, 2)
	
	// Forward stdin to server
	go func() {
		_, err := io.Copy(conn, os.Stdin)
		errChan <- err
	}()
	
	// Forward server responses to stdout
	go func() {
		_, err := io.Copy(os.Stdout, conn)
		errChan <- err
	}()
	
	// Wait for either direction to fail
	err := <-errChan
	return err
}

// runAsServer starts the server and accepts connections
func (cm *ConnectionManager) runAsServer() error {
	cm.isServer = true
	
	// Remove old pipe if exists
	os.Remove(pipePath)
	
	// Create named pipe listener
	listener, err := net.Listen("unix", pipePath)
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}
	cm.listener = listener
	defer listener.Close()
	defer os.Remove(pipePath)
	
	// Handle stdio as primary client
	go cm.handleStdioClient()
	
	// Accept additional connections
	go cm.acceptConnections()
	
	// Wait forever (or until interrupted)
	select {}
}

// handleStdioClient processes messages from stdin
func (cm *ConnectionManager) handleStdioClient() {
	client := &ClientConnection{
		ID:       "stdio",
		Reader:   bufio.NewReader(os.Stdin),
		Writer:   bufio.NewWriter(os.Stdout),
		LastSeen: time.Now(),
	}
	
	cm.clientsMu.Lock()
	cm.clients[client.ID] = client
	cm.clientsMu.Unlock()
	
	// Process messages
	for {
		line, err := client.Reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error reading from stdio: %v", err)
			continue
		}
		
		var msg MCPMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}
		
		// Update last seen
		client.LastSeen = time.Now()
		
		// Handle message
		response := cm.handleMessage(msg, client)
		
		// Send response if not a notification
		if msg.Method != "" && response.Jsonrpc == "" {
			continue
		}
		
		if err := cm.sendResponse(client, response); err != nil {
			log.Printf("Error sending response: %v", err)
		}
	}
	
	// Clean up
	cm.clientsMu.Lock()
	delete(cm.clients, client.ID)
	cm.clientsMu.Unlock()
}

// acceptConnections handles new client connections
func (cm *ConnectionManager) acceptConnections() {
	for {
		conn, err := cm.listener.Accept()
		if err != nil {
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		
		go cm.handleClient(conn)
	}
}

// handleClient processes a connected client
func (cm *ConnectionManager) handleClient(conn net.Conn) {
	defer conn.Close()
	
	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	client := &ClientConnection{
		ID:       clientID,
		Conn:     conn,
		Reader:   bufio.NewReader(conn),
		Writer:   bufio.NewWriter(conn),
		LastSeen: time.Now(),
	}
	
	cm.clientsMu.Lock()
	cm.clients[clientID] = client
	cm.clientsMu.Unlock()
	
	log.Printf("Client connected: %s", clientID)
	
	// Process messages
	decoder := json.NewDecoder(client.Reader)
	for {
		var msg MCPMessage
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error decoding message from %s: %v", clientID, err)
			continue
		}
		
		// Update last seen
		client.LastSeen = time.Now()
		
		// Handle message
		response := cm.handleMessage(msg, client)
		
		// Send response if not a notification
		if msg.Method != "" && response.Jsonrpc == "" {
			continue
		}
		
		if err := cm.sendResponse(client, response); err != nil {
			log.Printf("Error sending response to %s: %v", clientID, err)
			break
		}
	}
	
	// Clean up
	cm.clientsMu.Lock()
	delete(cm.clients, clientID)
	cm.clientsMu.Unlock()
	
	log.Printf("Client disconnected: %s", clientID)
}

// handleMessage routes messages to appropriate handlers
func (cm *ConnectionManager) handleMessage(msg MCPMessage, client *ClientConnection) MCPMessage {
	// Handle handoff requests
	if msg.Method == "handoff/request" {
		return cm.handleHandoffRequest(msg, client)
	}
	
	// Regular message handling
	return cm.server.handleMessage(msg)
}

// handleHandoffRequest processes handoff from stdio to pipe client
func (cm *ConnectionManager) handleHandoffRequest(msg MCPMessage, client *ClientConnection) MCPMessage {
	log.Printf("Handoff requested by %s", client.ID)
	
	// Send acknowledgment
	return MCPMessage{
		Jsonrpc: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"status": "connected",
			"server_info": map[string]interface{}{
				"uptime_seconds": time.Since(startTime).Seconds(),
				"active_clients": len(cm.clients),
				"memory_count":   len(cm.store.memories),
			},
		},
	}
}

// sendResponse sends a response to a client
func (cm *ConnectionManager) sendResponse(client *ClientConnection, response MCPMessage) error {
	responseBytes, err := json.Marshal(response)
	if err != nil {
		return err
	}
	
	client.Writer.Write(responseBytes)
	client.Writer.WriteByte('\n')
	return client.Writer.Flush()
}

// Helper to track server start time
var startTime = time.Now()