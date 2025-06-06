package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// Memory Types
type MemoryType string

const (
	ShortTerm  MemoryType = "short_term"
	LongTerm   MemoryType = "long_term"
	Episodic   MemoryType = "episodic"
	Semantic   MemoryType = "semantic"
	Procedural MemoryType = "procedural"
)

// Core Memory Structure
type Memory struct {
	ID          string                 `json:"id"`
	Type        MemoryType             `json:"type"`
	Content     string                 `json:"content"`
	Embedding   []float32              `json:"embedding,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
	Relations   []string               `json:"relations"`
	Timestamp   time.Time              `json:"timestamp"`
	LastAccess  time.Time              `json:"last_access"`
	AccessCount int                    `json:"access_count"`
	Importance  float32                `json:"importance"`
	Decay       float32                `json:"decay"`
}

// Graph-like structure for relationships
type MemoryRelation struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Type     string  `json:"type"`
	Strength float32 `json:"strength"`
}

// ScoredMemory represents a memory with its similarity score
type ScoredMemory struct {
	Memory *Memory
	Score  float32
}

// Main Memory Store
type MemoryStore struct {
	mu sync.RWMutex

	// Primary storage
	memories map[string]*Memory

	// Indexes for fast lookup
	typeIndex      map[MemoryType]map[string]*Memory
	timeIndex      *TimeIndex
	embeddingIndex *EmbeddingIndex

	// Relationship graph
	relations map[string][]*MemoryRelation

	// Memory management
	maxMemories   int
	decayInterval time.Duration
}

// Time-based index for temporal queries
type TimeIndex struct {
	mu      sync.RWMutex
	buckets map[string][]*Memory // hour buckets
}

// Embedding index for similarity search
type EmbeddingIndex struct {
	mu         sync.RWMutex
	embeddings map[string][]float32
	dimension  int
}

// MCP Server Tools
type MCPServer struct {
	store *MemoryStore
}

// Initialize the memory store
func NewMemoryStore(maxMemories int) *MemoryStore {
	store := &MemoryStore{
		memories:       make(map[string]*Memory),
		typeIndex:      make(map[MemoryType]map[string]*Memory),
		timeIndex:      &TimeIndex{buckets: make(map[string][]*Memory)},
		embeddingIndex: &EmbeddingIndex{embeddings: make(map[string][]float32), dimension: 384},
		relations:      make(map[string][]*MemoryRelation),
		maxMemories:    maxMemories,
		decayInterval:  5 * time.Minute,
	}

	// Initialize type indexes
	for _, t := range []MemoryType{ShortTerm, LongTerm, Episodic, Semantic, Procedural} {
		store.typeIndex[t] = make(map[string]*Memory)
	}

	// Start background processes
	go store.startDecayProcess()
	go store.startConsolidationProcess()

	return store
}

// Store a new memory
func (ms *MemoryStore) Store(memory *Memory) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Check capacity
	if len(ms.memories) >= ms.maxMemories {
		ms.evictLeastImportant()
	}

	// Store in primary map
	ms.memories[memory.ID] = memory

	// Update indexes
	ms.typeIndex[memory.Type][memory.ID] = memory
	ms.addToTimeIndex(memory)

	if memory.Embedding != nil {
		ms.embeddingIndex.mu.Lock()
		ms.embeddingIndex.embeddings[memory.ID] = memory.Embedding
		ms.embeddingIndex.mu.Unlock()
	}

	return nil
}

// Retrieve memories by various criteria
func (ms *MemoryStore) Query(criteria QueryCriteria) ([]*Memory, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var results []*Memory

	switch criteria.Type {
	case "similarity":
		results = ms.findSimilar(criteria.Embedding, criteria.Limit)
	case "temporal":
		results = ms.findTemporal(criteria.StartTime, criteria.EndTime)
	case "type":
		results = ms.findByType(criteria.MemoryType)
	case "related":
		results = ms.findRelated(criteria.MemoryID, criteria.Depth)
	default:
		results = ms.findByKeywords(criteria.Keywords)
	}

	// Update access patterns
	for _, mem := range results {
		mem.LastAccess = time.Now()
		mem.AccessCount++
	}

	return results, nil
}

// Find similar memories using embedding similarity
func (ms *MemoryStore) findSimilar(embedding []float32, limit int) []*Memory {
	var scores []ScoredMemory

	ms.embeddingIndex.mu.RLock()
	for id, emb := range ms.embeddingIndex.embeddings {
		if mem, ok := ms.memories[id]; ok {
			score := cosineSimilarity(embedding, emb)
			scores = append(scores, ScoredMemory{Memory: mem, Score: score})
		}
	}
	ms.embeddingIndex.mu.RUnlock()

	// Sort by score and return top k
	sortByScore(scores)

	results := make([]*Memory, 0, limit)
	for i := 0; i < len(scores) && i < limit; i++ {
		results = append(results, scores[i].Memory)
	}

	return results
}

// Find related memories using graph traversal
func (ms *MemoryStore) findRelated(memoryID string, depth int) []*Memory {
	visited := make(map[string]bool)
	queue := []string{memoryID}
	results := make([]*Memory, 0)

	for d := 0; d < depth && len(queue) > 0; d++ {
		nextQueue := []string{}

		for _, id := range queue {
			if visited[id] {
				continue
			}
			visited[id] = true

			if mem, ok := ms.memories[id]; ok && id != memoryID {
				results = append(results, mem)
			}

			// Add related memories to next queue
			if relations, ok := ms.relations[id]; ok {
				for _, rel := range relations {
					if !visited[rel.To] {
						nextQueue = append(nextQueue, rel.To)
					}
				}
			}
		}

		queue = nextQueue
	}

	return results
}

// Memory consolidation process
func (ms *MemoryStore) startConsolidationProcess() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ms.consolidateMemories()
	}
}

// Consolidate short-term memories into long-term
func (ms *MemoryStore) consolidateMemories() {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	shortTermMemories := ms.typeIndex[ShortTerm]

	for id, mem := range shortTermMemories {
		// Check if memory should be consolidated
		if mem.AccessCount > 3 || mem.Importance > 0.7 {
			// Convert to long-term memory
			mem.Type = LongTerm
			delete(ms.typeIndex[ShortTerm], id)
			ms.typeIndex[LongTerm][id] = mem

			// Strengthen relations
			if relations, ok := ms.relations[id]; ok {
				for _, rel := range relations {
					rel.Strength *= 1.2
				}
			}
		}
	}
}

// Memory decay process
func (ms *MemoryStore) startDecayProcess() {
	ticker := time.NewTicker(ms.decayInterval)
	defer ticker.Stop()

	for range ticker.C {
		ms.applyDecay()
	}
}

// Apply decay to memories
func (ms *MemoryStore) applyDecay() {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	now := time.Now()
	toRemove := []string{}

	for id, mem := range ms.memories {
		// Calculate decay based on time since last access
		timeSinceAccess := now.Sub(mem.LastAccess)
		decayFactor := float32(timeSinceAccess.Hours()) * mem.Decay

		// Reduce importance
		mem.Importance -= decayFactor

		// Mark for removal if importance too low
		if mem.Importance < 0.1 {
			toRemove = append(toRemove, id)
		}
	}

	// Remove decayed memories
	for _, id := range toRemove {
		ms.removeMemory(id)
	}
}

// MCP Tool Implementations

// Store a memory
func (mcp *MCPServer) StoreMemory(ctx context.Context, args StoreMemoryArgs) (*Memory, error) {
	memory := &Memory{
		ID:          generateID(),
		Type:        args.Type,
		Content:     args.Content,
		Embedding:   args.Embedding,
		Metadata:    args.Metadata,
		Relations:   args.Relations,
		Timestamp:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 0,
		Importance:  args.Importance,
		Decay:       0.01, // Default decay rate
	}

	err := mcp.store.Store(memory)
	return memory, err
}

// Query memories
func (mcp *MCPServer) QueryMemories(ctx context.Context, args QueryMemoryArgs) ([]*Memory, error) {
	criteria := QueryCriteria{
		Type:       args.QueryType,
		Keywords:   args.Keywords,
		MemoryType: MemoryType(args.MemoryType),
		Embedding:  args.Embedding,
		StartTime:  args.StartTime,
		EndTime:    args.EndTime,
		MemoryID:   args.MemoryID,
		Depth:      args.Depth,
		Limit:      args.Limit,
	}

	return mcp.store.Query(criteria)
}

// Create relation between memories
func (mcp *MCPServer) CreateRelation(ctx context.Context, args CreateRelationArgs) error {
	mcp.store.mu.Lock()
	defer mcp.store.mu.Unlock()

	relation := &MemoryRelation{
		From:     args.FromID,
		To:       args.ToID,
		Type:     args.RelationType,
		Strength: args.Strength,
	}

	mcp.store.relations[args.FromID] = append(mcp.store.relations[args.FromID], relation)

	return nil
}

// Get memory statistics
func (mcp *MCPServer) GetStats(ctx context.Context) (map[string]interface{}, error) {
	mcp.store.mu.RLock()
	defer mcp.store.mu.RUnlock()

	stats := map[string]interface{}{
		"total_memories": len(mcp.store.memories),
		"by_type": map[string]int{
			"short_term": len(mcp.store.typeIndex[ShortTerm]),
			"long_term":  len(mcp.store.typeIndex[LongTerm]),
			"episodic":   len(mcp.store.typeIndex[Episodic]),
			"semantic":   len(mcp.store.typeIndex[Semantic]),
			"procedural": len(mcp.store.typeIndex[Procedural]),
		},
		"total_relations": len(mcp.store.relations),
		"capacity_used":   float32(len(mcp.store.memories)) / float32(mcp.store.maxMemories),
	}

	return stats, nil
}

// Helper functions

func generateID() string {
	return fmt.Sprintf("mem_%d", time.Now().UnixNano())
}

func cosineSimilarity(a, b []float32) float32 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	return dot / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float32) float32 {
	return float32(math.Sqrt(float64(x)))
}

// Additional helper types

type QueryCriteria struct {
	Type       string
	Keywords   []string
	MemoryType MemoryType
	Embedding  []float32
	StartTime  time.Time
	EndTime    time.Time
	MemoryID   string
	Depth      int
	Limit      int
}

type StoreMemoryArgs struct {
	Type       MemoryType             `json:"type"`
	Content    string                 `json:"content"`
	Embedding  []float32              `json:"embedding,omitempty"`
	Metadata   map[string]interface{} `json:"metadata"`
	Relations  []string               `json:"relations"`
	Importance float32                `json:"importance"`
}

type QueryMemoryArgs struct {
	QueryType  string    `json:"query_type"`
	Keywords   []string  `json:"keywords,omitempty"`
	MemoryType string    `json:"memory_type,omitempty"`
	Embedding  []float32 `json:"embedding,omitempty"`
	StartTime  time.Time `json:"start_time,omitempty"`
	EndTime    time.Time `json:"end_time,omitempty"`
	MemoryID   string    `json:"memory_id,omitempty"`
	Depth      int       `json:"depth,omitempty"`
	Limit      int       `json:"limit,omitempty"`
}

type CreateRelationArgs struct {
	FromID       string  `json:"from_id"`
	ToID         string  `json:"to_id"`
	RelationType string  `json:"relation_type"`
	Strength     float32 `json:"strength"`
}
