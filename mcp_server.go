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
		result = mcp.GetWiki()

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

// Get wiki documentation
func (mcp *MCPServer) GetWiki() string {
	return `# Memory System Documentation

## Overview
This is a cognitive memory system that provides fast, in-memory storage and retrieval for AI agents. It models human-like memory with different types, automatic consolidation, decay, and relationship tracking.

## Multi-Client Memory Sharing (NEW)

### Enable Shared Mode
Run the server with --enable-sharing flag to allow multiple clients to share the same memory space:

	./mcp-memory-server --enable-sharing

### How It Works
1. **First Client**: Starts the server and creates a named pipe at /tmp/mcp-memory-server.pipe
2. **Additional Clients**: Automatically detect the running server and connect via the pipe
3. **Shared Memory**: All clients access the same in-memory store
4. **Handoff Protocol**: Clients announce themselves and can transfer connections

### Benefits
- Share memories between Claude Desktop and Claude Code
- Maintain context across different AI interfaces
- No external dependencies or persistence required
- Automatic server discovery

### Example Configuration
Terminal 1 - Claude Desktop config:
	{
	  "mcpServers": {
	    "memory": {
	      "command": "/path/to/mcp-memory-server",
	      "args": ["--enable-sharing", "--max-memories", "1000"]
	    }
	  }
	}

Terminal 2 - Claude Code config (will connect to same server):
	{
	  "mcpServers": {
	    "memory": {
	      "command": "/path/to/mcp-memory-server",
	      "args": ["--enable-sharing", "--max-memories", "1000"]
	    }
	  }
	}

## Memory Types

### short_term
- Temporary information from current conversation
- Auto-promotes to long_term if accessed frequently
- Decays quickly if not accessed

### long_term
- Important facts to remember permanently
- Consolidated from frequently accessed short_term memories
- Slower decay rate

### episodic
- Specific events or interactions
- "User asked about X at time Y"
- Maintains temporal context

### semantic
- Facts, knowledge, and concepts
- "User prefers Python over Java"
- Core knowledge base

### procedural
- How-to knowledge and patterns
- "Always format code with 4 spaces for this user"
- Guides future behavior

## Core Operations

### store_memory
Stores new information with cognitive type and importance.

Required parameters:
- type: Memory type (short_term, long_term, episodic, semantic, procedural)
- content: The information to store

Optional parameters:
- importance: 0.0-1.0 score (default: 0.5)
  - 0.9-1.0: Critical (passwords, key preferences)
  - 0.7-0.8: Important (project details)
  - 0.5-0.6: Useful (general interests)
  - 0.1-0.4: Minor (small talk)
- metadata: JSON object with additional context

### query_memories
Retrieves memories using different strategies.

Required parameters:
- query_type: Search strategy
  - keywords: Search content for terms
  - type: Get all of specific type
  - temporal: Find within time range
  - related: Traverse relationships
  - similarity: Vector similarity (if embeddings)

Optional parameters:
- keywords: Array of search terms
- memory_type: Filter by type
- limit: Max results (default: 10)
- start_time/end_time: For temporal queries
- memory_id: Starting point for related queries
- depth: Traversal depth for related queries

### create_relation
Links memories with typed relationships.

Required parameters:
- from_id: Source memory ID
- to_id: Target memory ID
- relation_type: Type of relationship

Optional parameters:
- strength: 0.0-1.0 (default: 0.5)

Relation types:
- related_to: General association
- leads_to: Causal/temporal sequence
- derived_from: Conclusions from facts
- influences: Affects handling
- part_of: Component relationship

### get_stats
Returns system statistics. No parameters required.

Returns:
- total_memories: Count of all memories
- by_type: Breakdown by memory type
- total_relations: Number of relationships
- capacity_used: Percentage of max capacity

## Best Practices

### What to Remember
✓ User identity and background
✓ Preferences (technical, style, tools)
✓ Current projects and context
✓ Problems and their solutions
✓ Behavioral patterns
✓ Important agreements

### What NOT to Remember
✗ Sensitive data (passwords, keys, PII)
✗ Large code blocks or file contents
✗ Information user asks to forget
✗ Low-value repetitive data

### Memory Patterns

#### Learning About Users
1. Store episodic memory of introduction (0.9 importance)
2. Store semantic facts about them (0.8 importance)
3. Create relations between identity and preferences
4. Store procedural patterns for interaction

#### Project Tracking
1. Query existing project memories
2. Store new project details as semantic
3. Link updates with "part_of" relations
4. Track progress with episodic memories

#### Problem Solving
1. Store problem as episodic
2. Store solution as semantic
3. Link with "solved_by" relation
4. Create procedural memory for pattern

### Query Strategy
1. Start broad with keywords
2. Refine with type filters if needed
3. Use related queries for context
4. Check temporal for recent items

### Memory Lifecycle
1. New info → short_term
2. Frequent access → auto-promote to long_term
3. Unused memories gradually decay
4. Critical memories (0.9+) decay slowly

## Technical Details

### Performance
- Sub-millisecond operations
- In-memory storage (no disk I/O)
- Automatic memory management
- Configurable capacity limits

### Automatic Features
- Memory consolidation (short→long term)
- Importance-based eviction
- Time-based decay
- Access pattern tracking

### Memory Structure
Each memory contains:
- Unique ID (timestamp-based)
- Type, content, importance
- Optional metadata and embeddings
- Access count and last access time
- Decay rate
- Relations to other memories

## Integration Tips

### Before Conversations
1. Query for user context
2. Load relevant project memories
3. Check recent interactions

### During Conversations
1. Store important new information
2. Update access on retrieved memories
3. Create relations as patterns emerge

### After Conversations
1. Store conversation summary
2. Extract key learnings
3. Update procedural patterns

Remember: This system helps maintain context across interactions. Use it thoughtfully to provide personalized, contextual assistance while respecting privacy and relevance.`
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
