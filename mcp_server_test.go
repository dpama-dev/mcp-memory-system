package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// Test MCP message handling
func TestHandleInitialize(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	// Create initialize request
	msg := MCPMessage{
		Jsonrpc: "2.0",
		ID:      1,
		Method:  "initialize",
	}
	
	response := server.handleMessage(msg)
	
	// Verify response
	if response.Jsonrpc != "2.0" {
		t.Errorf("Expected jsonrpc 2.0, got %s", response.Jsonrpc)
	}
	
	if response.Error != nil {
		t.Errorf("Expected no error, got %v", response.Error)
	}
	
	// Check result type
	result, ok := response.Result.(InitializeResult)
	if !ok {
		t.Fatal("Response result is not InitializeResult")
	}
	
	// Verify protocol version
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("Expected protocol version 2024-11-05, got %s", result.ProtocolVersion)
	}
	
	// Verify capabilities
	if result.Capabilities.Tools == nil || !result.Capabilities.Tools.ListChanged {
		t.Error("Tools capability not properly set")
	}
	
	if result.Capabilities.Resources == nil || !result.Capabilities.Resources.Subscribe {
		t.Error("Resources capability not properly set")
	}
}

// Test tools list
func TestHandleToolsList(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	msg := MCPMessage{
		Jsonrpc: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	
	response := server.handleMessage(msg)
	
	if response.Error != nil {
		t.Fatalf("Expected no error, got %v", response.Error)
	}
	
	// Extract tools from response
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Response result is not a map")
	}
	
	tools, ok := result["tools"].([]Tool)
	if !ok {
		t.Fatal("Tools not found in response")
	}
	
	// Verify we have the expected tools
	expectedTools := []string{"store_memory", "query_memories", "create_relation", "get_stats", "wiki"}
	
	if len(tools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(tools))
	}
	
	// Verify each tool exists
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}
	
	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("Expected tool %s not found", expected)
		}
	}
}

// Test store_memory tool call
func TestHandleStoreMemoryTool(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	// Create tool call arguments
	args := StoreMemoryArgs{
		Type:       ShortTerm,
		Content:    "Test memory content",
		Importance: 0.7,
		Metadata: map[string]interface{}{
			"source": "test",
		},
	}
	
	argsJSON, _ := json.Marshal(args)
	params := json.RawMessage(`{"name": "store_memory", "arguments": ` + string(argsJSON) + `}`)
	
	msg := MCPMessage{
		Jsonrpc: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params:  params,
	}
	
	response := server.handleMessage(msg)
	
	if response.Error != nil {
		t.Fatalf("Expected no error, got %v", response.Error)
	}
	
	// Verify memory was stored
	store.mu.RLock()
	memCount := len(store.memories)
	store.mu.RUnlock()
	
	if memCount != 1 {
		t.Errorf("Expected 1 memory stored, got %d", memCount)
	}
}

// Test store_memory validation errors
func TestStoreMemoryValidation(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	tests := []struct {
		name    string
		args    StoreMemoryArgs
		wantErr bool
	}{
		{
			name: "empty content",
			args: StoreMemoryArgs{
				Type:       ShortTerm,
				Content:    "",
				Importance: 0.5,
			},
			wantErr: true,
		},
		{
			name: "invalid memory type",
			args: StoreMemoryArgs{
				Type:       "invalid_type",
				Content:    "test",
				Importance: 0.5,
			},
			wantErr: true,
		},
		{
			name: "valid memory",
			args: StoreMemoryArgs{
				Type:       Episodic,
				Content:    "Valid memory",
				Importance: 0.8,
			},
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.StoreMemory(context.Background(), tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("StoreMemory() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test query_memories tool call
func TestHandleQueryMemoriesTool(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	// Store some test memories first
	memories := []StoreMemoryArgs{
		{Type: ShortTerm, Content: "The quick brown fox", Importance: 0.5},
		{Type: LongTerm, Content: "Important information", Importance: 0.8},
		{Type: Episodic, Content: "User clicked button", Importance: 0.3},
	}
	
	for _, args := range memories {
		if _, err := server.StoreMemory(context.Background(), args); err != nil {
			t.Fatalf("Failed to store test memory: %v", err)
		}
	}
	
	// Test keyword query
	queryArgs := QueryMemoryArgs{
		QueryType: "keywords",
		Keywords:  []string{"quick"},
		Limit:     10,
	}
	
	results, err := server.QueryMemories(context.Background(), queryArgs)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	
	if len(results) != 1 {
		t.Errorf("Expected 1 result for 'quick', got %d", len(results))
	}
	
	// Test type query
	queryArgs = QueryMemoryArgs{
		QueryType:  "type",
		MemoryType: "long_term",
		Limit:      10,
	}
	
	results, err = server.QueryMemories(context.Background(), queryArgs)
	if err != nil {
		t.Fatalf("Type query failed: %v", err)
	}
	
	if len(results) != 1 {
		t.Errorf("Expected 1 long_term memory, got %d", len(results))
	}
}

// Test create_relation tool
func TestCreateRelation(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	// Store two memories first
	mem1, _ := server.StoreMemory(context.Background(), StoreMemoryArgs{
		Type:       ShortTerm,
		Content:    "First memory",
		Importance: 0.5,
	})
	
	mem2, _ := server.StoreMemory(context.Background(), StoreMemoryArgs{
		Type:       ShortTerm,
		Content:    "Second memory",
		Importance: 0.5,
	})
	
	// Create relation between them
	args := CreateRelationArgs{
		FromID:       mem1.ID,
		ToID:         mem2.ID,
		RelationType: "related_to",
		Strength:     0.8,
	}
	
	err := server.CreateRelation(context.Background(), args)
	if err != nil {
		t.Fatalf("Failed to create relation: %v", err)
	}
	
	// Verify relation exists
	store.mu.RLock()
	relations, exists := store.relations[mem1.ID]
	store.mu.RUnlock()
	
	if !exists || len(relations) != 1 {
		t.Error("Relation not created properly")
	}
	
	if relations[0].To != mem2.ID {
		t.Error("Relation points to wrong memory")
	}
}

// Test relation validation
func TestCreateRelationValidation(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	// Store one memory
	mem1, _ := server.StoreMemory(context.Background(), StoreMemoryArgs{
		Type:       ShortTerm,
		Content:    "First memory",
		Importance: 0.5,
	})
	
	tests := []struct {
		name    string
		args    CreateRelationArgs
		wantErr bool
	}{
		{
			name: "empty from_id",
			args: CreateRelationArgs{
				FromID:       "",
				ToID:         mem1.ID,
				RelationType: "related_to",
			},
			wantErr: true,
		},
		{
			name: "empty to_id",
			args: CreateRelationArgs{
				FromID:       mem1.ID,
				ToID:         "",
				RelationType: "related_to",
			},
			wantErr: true,
		},
		{
			name: "empty relation_type",
			args: CreateRelationArgs{
				FromID:       mem1.ID,
				ToID:         mem1.ID,
				RelationType: "",
			},
			wantErr: true,
		},
		{
			name: "non-existent from memory",
			args: CreateRelationArgs{
				FromID:       "non-existent",
				ToID:         mem1.ID,
				RelationType: "related_to",
			},
			wantErr: true,
		},
		{
			name: "non-existent to memory",
			args: CreateRelationArgs{
				FromID:       mem1.ID,
				ToID:         "non-existent",
				RelationType: "related_to",
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.CreateRelation(context.Background(), tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRelation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test get_stats tool
func TestGetStats(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	// Store memories of different types
	types := []MemoryType{ShortTerm, ShortTerm, LongTerm, Episodic, Semantic, Procedural}
	for i, memType := range types {
		_, err := server.StoreMemory(context.Background(), StoreMemoryArgs{
			Type:       memType,
			Content:    fmt.Sprintf("Test memory %d", i),
			Importance: 0.5,
		})
		if err != nil {
			t.Fatalf("Failed to store memory %d: %v", i, err)
		}
		// Small delay to ensure unique IDs
		time.Sleep(time.Microsecond)
	}
	
	stats, err := server.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	
	// Verify total count
	totalMemories, ok := stats["total_memories"].(int)
	if !ok {
		t.Fatal("total_memories not found in stats")
	}
	
	if totalMemories != len(types) {
		t.Errorf("Expected %d total memories, got %d", len(types), totalMemories)
	}
	
	// Verify type breakdown
	byType, ok := stats["by_type"].(map[string]int)
	if !ok {
		t.Fatal("by_type not found in stats")
	}
	
	if byType["short_term"] != 2 {
		t.Errorf("Expected 2 short_term memories, got %d", byType["short_term"])
	}
	
	// Verify capacity
	capacityUsed, ok := stats["capacity_used"].(float32)
	if !ok {
		t.Fatal("capacity_used not found in stats")
	}
	
	expectedCapacity := float32(len(types)) / float32(store.maxMemories)
	if capacityUsed != expectedCapacity {
		t.Errorf("Expected capacity %f, got %f", expectedCapacity, capacityUsed)
	}
}

// Test wiki tool
func TestGetWiki(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	wiki := server.GetWiki()
	
	// Verify wiki contains expected sections
	expectedSections := []string{
		"# Memory System Documentation",
		"## Overview",
		"## Multi-Client Memory Sharing",
		"## Memory Types",
		"## Core Operations",
		"## Best Practices",
	}
	
	for _, section := range expectedSections {
		if !contains(wiki, section) {
			t.Errorf("Wiki missing expected section: %s", section)
		}
	}
}

// Test invalid tool call
func TestInvalidToolCall(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	params := json.RawMessage(`{"name": "non_existent_tool", "arguments": {}}`)
	
	msg := MCPMessage{
		Jsonrpc: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params:  params,
	}
	
	response := server.handleMessage(msg)
	
	if response.Error == nil {
		t.Error("Expected error for invalid tool, got nil")
	}
	
	if response.Error.Code != -32601 {
		t.Errorf("Expected error code -32601, got %d", response.Error.Code)
	}
}

// Test invalid method
func TestInvalidMethod(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	msg := MCPMessage{
		Jsonrpc: "2.0",
		ID:      5,
		Method:  "invalid/method",
	}
	
	response := server.handleMessage(msg)
	
	if response.Error == nil {
		t.Error("Expected error for invalid method, got nil")
	}
	
	if response.Error.Code != -32601 {
		t.Errorf("Expected error code -32601, got %d", response.Error.Code)
	}
}

// Test resources list
func TestHandleResourcesList(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	msg := MCPMessage{
		Jsonrpc: "2.0",
		ID:      6,
		Method:  "resources/list",
	}
	
	response := server.handleMessage(msg)
	
	if response.Error != nil {
		t.Fatalf("Expected no error, got %v", response.Error)
	}
	
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatal("Response result is not a map")
	}
	
	resources, ok := result["resources"].([]map[string]string)
	if !ok {
		t.Fatal("Resources not found in response")
	}
	
	// Verify we have the expected resources
	if len(resources) != 2 {
		t.Errorf("Expected 2 resources, got %d", len(resources))
	}
	
	// Check resource URIs
	expectedURIs := []string{"memory://stats", "memory://graph"}
	for i, resource := range resources {
		if resource["uri"] != expectedURIs[i] {
			t.Errorf("Expected URI %s, got %s", expectedURIs[i], resource["uri"])
		}
	}
}

// Test malformed JSON in tool arguments
func TestMalformedToolArguments(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	// Malformed JSON in arguments
	params := json.RawMessage(`{"name": "store_memory", "arguments": {invalid json}}`)
	
	msg := MCPMessage{
		Jsonrpc: "2.0",
		ID:      7,
		Method:  "tools/call",
		Params:  params,
	}
	
	response := server.handleMessage(msg)
	
	if response.Error == nil {
		t.Error("Expected error for malformed arguments, got nil")
	}
	
	if response.Error.Code != -32602 {
		t.Errorf("Expected error code -32602, got %d", response.Error.Code)
	}
}

// Helper function for string contains
func contains(text, substring string) bool {
	return len(substring) > 0 && len(text) >= len(substring) &&
		findSubstring(text, substring)
}

func findSubstring(text, pattern string) bool {
	for i := 0; i <= len(text)-len(pattern); i++ {
		if text[i:i+len(pattern)] == pattern {
			return true
		}
	}
	return false
}