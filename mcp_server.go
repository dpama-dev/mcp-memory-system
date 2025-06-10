package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"mcp-memory-system/internal/docs"
)

// MCP Protocol Messages
type MCPMessage struct {
	Jsonrpc string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *MCPError       `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP Initialize Response
type InitializeResult struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    MCPCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo      `json:"serverInfo"`
}

type MCPCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe"`
	ListChanged bool `json:"listChanged"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool definitions
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
}

// Main MCP server implementation
func main() {
	// Initialize memory store
	config := LoadConfig()
	InitializeMemoryLimits(config)

	store := NewMemoryStore(config.MaxMemories)
	server := &MCPServer{store: store}

	log.SetOutput(os.Stderr) // Log to stderr to avoid interfering with protocol

	// Set up graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, cleaning up...")
		store.Shutdown()
		os.Exit(0)
	}()

	// Use connection manager for multi-client support
	if config.EnableSharing {
		connManager := NewConnectionManager(store, server)
		if err := connManager.Start(); err != nil {
			log.Fatalf("Failed to start connection manager: %v", err)
		}
	} else {
		// Original stdio-only mode
		runStdioMode(server)
	}
}

// runStdioMode runs the server in traditional stdio mode
func runStdioMode(server *MCPServer) {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	log.Println("Memory MCP Server started (stdio mode)")

	// Main message loop
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Error reading: %v", err)
			continue
		}

		var msg MCPMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			log.Printf("Error parsing message: %v", err)
			continue
		}

		response := server.handleMessage(msg)

		// Don't send response for notifications
		if msg.Method != "" && response.Jsonrpc == "" {
			continue
		}

		responseBytes, err := json.Marshal(response)
		if err != nil {
			log.Printf("Error marshaling response: %v", err)
			continue
		}

		writer.Write(responseBytes)
		writer.WriteByte('\n')
		writer.Flush()
	}
}

func (mcp *MCPServer) handleMessage(msg MCPMessage) MCPMessage {
	switch msg.Method {
	case "initialize":
		return mcp.handleInitialize(msg)
	case "tools/list":
		return mcp.handleToolsList(msg)
	case "tools/call":
		return mcp.handleToolCall(msg)
	case "resources/list":
		return mcp.handleResourcesList(msg)
	case "resources/read":
		return mcp.handleResourceRead(msg)
	case "notifications/initialized":
		// Client has initialized, just acknowledge
		log.Println("Client initialized successfully")
		return MCPMessage{} // Empty response for notifications
	default:
		return MCPMessage{
			Jsonrpc: "2.0",
			ID:      msg.ID,
			Error: &MCPError{
				Code:    -32601,
				Message: "Method not found",
			},
		}
	}
}

func (mcp *MCPServer) handleInitialize(msg MCPMessage) MCPMessage {
	return MCPMessage{
		Jsonrpc: "2.0",
		ID:      msg.ID,
		Result: InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: MCPCapabilities{
				Tools: &ToolsCapability{
					ListChanged: true,
				},
				Resources: &ResourcesCapability{
					Subscribe:   true,
					ListChanged: true,
				},
			},
			ServerInfo: ServerInfo{
				Name:    "mcp-memory-server",
				Version: "1.0.0",
			},
		},
	}
}

func (mcp *MCPServer) handleToolsList(msg MCPMessage) MCPMessage {
	tools := []Tool{
		{
			Name:        "store_memory",
			Description: "Store a new memory with content, type, and metadata",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"type": {
						Type:        "string",
						Description: "Memory type",
						Enum:        []string{"short_term", "long_term", "episodic", "semantic", "procedural"},
					},
					"content": {
						Type:        "string",
						Description: "The memory content",
					},
					"metadata": {
						Type:        "object",
						Description: "Additional metadata",
					},
					"importance": {
						Type:        "number",
						Description: "Importance score (0-1)",
					},
				},
				Required: []string{"type", "content"},
			},
		},
		{
			Name:        "query_memories",
			Description: "Query memories by various criteria",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"query_type": {
						Type:        "string",
						Description: "Type of query",
						Enum:        []string{"similarity", "temporal", "type", "related", "keywords"},
					},
					"keywords": {
						Type:        "array",
						Description: "Keywords to search for",
					},
					"memory_type": {
						Type:        "string",
						Description: "Filter by memory type",
					},
					"limit": {
						Type:        "integer",
						Description: "Maximum results to return",
					},
				},
				Required: []string{"query_type"},
			},
		},
		{
			Name:        "create_relation",
			Description: "Create a relation between two memories",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"from_id": {
						Type:        "string",
						Description: "Source memory ID",
					},
					"to_id": {
						Type:        "string",
						Description: "Target memory ID",
					},
					"relation_type": {
						Type:        "string",
						Description: "Type of relation",
					},
					"strength": {
						Type:        "number",
						Description: "Relation strength (0-1)",
					},
				},
				Required: []string{"from_id", "to_id", "relation_type"},
			},
		},
		{
			Name:        "get_stats",
			Description: "Get memory store statistics",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
				Required:   []string{},
			},
		},
		{
			Name:        "wiki",
			Description: "Get comprehensive documentation on how to use the memory system",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]Property{},
				Required:   []string{},
			},
		},
	}

	return MCPMessage{
		Jsonrpc: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

func (mcp *MCPServer) handleToolCall(msg MCPMessage) MCPMessage {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return MCPMessage{
			Jsonrpc: "2.0",
			ID:      msg.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: "Invalid params",
			},
		}
	}

	var result interface{}
	var err error

	switch params.Name {
	case "store_memory":
		var args StoreMemoryArgs
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return MCPMessage{
				Jsonrpc: "2.0",
				ID:      msg.ID,
				Error: &MCPError{
					Code:    -32602,
					Message: fmt.Sprintf("Invalid arguments for store_memory: %v", err),
				},
			}
		}
		result, err = mcp.StoreMemory(nil, args)

	case "query_memories":
		var args QueryMemoryArgs
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return MCPMessage{
				Jsonrpc: "2.0",
				ID:      msg.ID,
				Error: &MCPError{
					Code:    -32602,
					Message: fmt.Sprintf("Invalid arguments for query_memories: %v", err),
				},
			}
		}
		result, err = mcp.QueryMemories(nil, args)

	case "create_relation":
		var args CreateRelationArgs
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return MCPMessage{
				Jsonrpc: "2.0",
				ID:      msg.ID,
				Error: &MCPError{
					Code:    -32602,
					Message: fmt.Sprintf("Invalid arguments for create_relation: %v", err),
				},
			}
		}
		err = mcp.CreateRelation(nil, args)
		result = map[string]string{"status": "success"}

	case "get_stats":
		result, err = mcp.GetStats(nil)

	case "wiki":
		result = docs.GetWiki()

	default:
		return MCPMessage{
			Jsonrpc: "2.0",
			ID:      msg.ID,
			Error: &MCPError{
				Code:    -32601,
				Message: "Unknown tool",
			},
		}
	}

	if err != nil {
		return MCPMessage{
			Jsonrpc: "2.0",
			ID:      msg.ID,
			Error: &MCPError{
				Code:    -32603,
				Message: err.Error(),
			},
		}
	}

	return MCPMessage{
		Jsonrpc: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": formatResult(result),
				},
			},
		},
	}
}

func (mcp *MCPServer) handleResourcesList(msg MCPMessage) MCPMessage {
	resources := []map[string]string{
		{
			"uri":         "memory://stats",
			"name":        "Memory Statistics",
			"description": "Current memory store statistics",
			"mimeType":    "application/json",
		},
		{
			"uri":         "memory://graph",
			"name":        "Memory Graph",
			"description": "Memory relationship graph",
			"mimeType":    "application/json",
		},
	}

	return MCPMessage{
		Jsonrpc: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"resources": resources,
		},
	}
}

func (mcp *MCPServer) handleResourceRead(msg MCPMessage) MCPMessage {
	var params struct {
		URI string `json:"uri"`
	}

	json.Unmarshal(msg.Params, &params)

	var content interface{}

	switch params.URI {
	case "memory://stats":
		stats, _ := mcp.GetStats(nil)
		content = stats

	case "memory://graph":
		// Return a simplified graph representation
		mcp.store.mu.RLock()
		graph := map[string]interface{}{
			"nodes": len(mcp.store.memories),
			"edges": len(mcp.store.relations),
		}
		mcp.store.mu.RUnlock()
		content = graph

	default:
		return MCPMessage{
			Jsonrpc: "2.0",
			ID:      msg.ID,
			Error: &MCPError{
				Code:    -32602,
				Message: "Unknown resource",
			},
		}
	}

	return MCPMessage{
		Jsonrpc: "2.0",
		ID:      msg.ID,
		Result: map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      params.URI,
					"mimeType": "application/json",
					"text":     formatResult(content),
				},
			},
		},
	}
}


// Helper functions

func formatResult(v interface{}) string {
	bytes, _ := json.MarshalIndent(v, "", "  ")
	return string(bytes)
}

func sortByScore(scores []ScoredMemory) {
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
}

func (ms *MemoryStore) addToTimeIndex(memory *Memory) {
	bucket := memory.Timestamp.Format("2006-01-02-15")
	ms.timeIndex.mu.Lock()
	ms.timeIndex.buckets[bucket] = append(ms.timeIndex.buckets[bucket], memory)
	ms.timeIndex.mu.Unlock()
}

func (ms *MemoryStore) findTemporal(start, end time.Time) []*Memory {
	var results []*Memory

	ms.timeIndex.mu.RLock()
	for _, memories := range ms.timeIndex.buckets {
		for _, mem := range memories {
			if mem.Timestamp.After(start) && mem.Timestamp.Before(end) {
				results = append(results, mem)
			}
		}
	}
	ms.timeIndex.mu.RUnlock()

	return results
}

func (ms *MemoryStore) findByType(memType MemoryType) []*Memory {
	var results []*Memory

	if memories, ok := ms.typeIndex[memType]; ok {
		for _, mem := range memories {
			results = append(results, mem)
		}
	}

	return results
}

func (ms *MemoryStore) findByKeywords(keywords []string) []*Memory {
	resultMap := make(map[string]*Memory)

	ms.keywordIndex.mu.RLock()
	defer ms.keywordIndex.mu.RUnlock()

	// Use keyword index for fast lookup
	for _, keyword := range keywords {
		lowerKeyword := strings.ToLower(keyword)
		if memories, exists := ms.keywordIndex.index[lowerKeyword]; exists {
			for id, mem := range memories {
				resultMap[id] = mem
			}
		}
	}

	// Convert map to slice
	results := make([]*Memory, 0, len(resultMap))
	for _, mem := range resultMap {
		results = append(results, mem)
	}

	return results
}

func (ms *MemoryStore) evictLeastImportant() {
	var leastImportant *Memory
	var leastID string

	for id, mem := range ms.memories {
		if leastImportant == nil || mem.Importance < leastImportant.Importance {
			leastImportant = mem
			leastID = id
		}
	}

	if leastID != "" {
		ms.removeMemory(leastID)
	}
}

func (ms *MemoryStore) removeMemory(id string) {
	if mem, ok := ms.memories[id]; ok {
		delete(ms.memories, id)
		delete(ms.typeIndex[mem.Type], id)
		delete(ms.relations, id)

		ms.embeddingIndex.mu.Lock()
		delete(ms.embeddingIndex.embeddings, id)
		ms.embeddingIndex.mu.Unlock()

		// Remove from keyword index
		ms.removeFromKeywordIndex(mem)

		// Clean up old time buckets
		ms.cleanupTimeBuckets()
	}
}

// Legacy contains function - now using keyword index for better performance
