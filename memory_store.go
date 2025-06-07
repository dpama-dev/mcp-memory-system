package main

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
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

// Keyword index for fast text search
type KeywordIndex struct {
	mu    sync.RWMutex
	index map[string]map[string]*Memory // keyword -> memoryID -> Memory
}

// Priority queue for top-K similarity search
type ScoredMemoryHeap []*ScoredMemory

func (h ScoredMemoryHeap) Len() int           { return len(h) }
func (h ScoredMemoryHeap) Less(i, j int) bool { return h[i].Score < h[j].Score }
func (h ScoredMemoryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *ScoredMemoryHeap) Push(x interface{}) {
	*h = append(*h, x.(*ScoredMemory))
}

func (h *ScoredMemoryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
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
	keywordIndex   *KeywordIndex

	// Relationship graph
	relations map[string][]*MemoryRelation

	// Memory management
	maxMemories   int
	decayInterval time.Duration
	shutdownChan  chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
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
		keywordIndex:   &KeywordIndex{index: make(map[string]map[string]*Memory)},
		relations:      make(map[string][]*MemoryRelation),
		maxMemories:    maxMemories,
		decayInterval:  5 * time.Minute,
		shutdownChan:   make(chan struct{}),
	}

	// Set up context for graceful shutdown
	store.ctx, store.cancel = context.WithCancel(context.Background())

	// Initialize type indexes
	for _, t := range []MemoryType{ShortTerm, LongTerm, Episodic, Semantic, Procedural} {
		store.typeIndex[t] = make(map[string]*Memory)
	}

	// Start background processes
	go store.startDecayProcess()
	go store.startConsolidationProcess()

	return store
}

// Shutdown gracefully stops all background processes
func (ms *MemoryStore) Shutdown() {
	ms.cancel()
	close(ms.shutdownChan)
}

// Store a new memory with validation
func (ms *MemoryStore) Store(memory *Memory) error {
	// Validate input
	if memory == nil {
		return errors.New("memory cannot be nil")
	}
	if memory.ID == "" {
		return errors.New("memory ID cannot be empty")
	}
	if memory.Content == "" {
		return errors.New("memory content cannot be empty")
	}
	if memory.Importance < 0 || memory.Importance > 1 {
		return errors.New("memory importance must be between 0 and 1")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Check for duplicate ID
	if _, exists := ms.memories[memory.ID]; exists {
		return fmt.Errorf("memory with ID %s already exists", memory.ID)
	}

	// Check capacity
	if len(ms.memories) >= ms.maxMemories {
		ms.evictLeastImportant()
	}

	// Store in primary map
	ms.memories[memory.ID] = memory

	// Update indexes
	ms.typeIndex[memory.Type][memory.ID] = memory
	ms.addToTimeIndex(memory)
	ms.addToKeywordIndex(memory)

	if memory.Embedding != nil {
		// Normalize embedding for faster cosine similarity
		normalizedEmbedding := normalizeVector(memory.Embedding)
		ms.embeddingIndex.mu.Lock()
		ms.embeddingIndex.embeddings[memory.ID] = normalizedEmbedding
		ms.embeddingIndex.mu.Unlock()
	}

	return nil
}

// Retrieve memories by various criteria with validation
func (ms *MemoryStore) Query(criteria QueryCriteria) ([]*Memory, error) {
	// Validate query criteria
	if criteria.Type == "" {
		return nil, errors.New("query type cannot be empty")
	}
	if criteria.Limit < 0 {
		return nil, errors.New("query limit cannot be negative")
	}
	if criteria.Limit == 0 {
		criteria.Limit = 10 // Default limit
	}
	if criteria.Limit > 1000 {
		return nil, errors.New("query limit cannot exceed 1000")
	}

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

// Find similar memories using embedding similarity with heap-based top-K
func (ms *MemoryStore) findSimilar(embedding []float32, limit int) []*Memory {
	// Normalize query embedding
	normalizedQuery := normalizeVector(embedding)

	// Use min-heap to maintain top-K efficiently
	h := &ScoredMemoryHeap{}
	heap.Init(h)

	ms.embeddingIndex.mu.RLock()
	for id, emb := range ms.embeddingIndex.embeddings {
		if mem, ok := ms.memories[id]; ok {
			// Since both vectors are normalized, dot product = cosine similarity
			score := dotProduct(normalizedQuery, emb)

			if h.Len() < limit {
				heap.Push(h, &ScoredMemory{Memory: mem, Score: score})
			} else if score > (*h)[0].Score {
				heap.Pop(h)
				heap.Push(h, &ScoredMemory{Memory: mem, Score: score})
			}
		}
	}
	ms.embeddingIndex.mu.RUnlock()

	// Extract results in descending order
	results := make([]*Memory, 0, h.Len())
	for h.Len() > 0 {
		scored := heap.Pop(h).(*ScoredMemory)
		results = append([]*Memory{scored.Memory}, results...)
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

// Memory consolidation process with graceful shutdown
func (ms *MemoryStore) startConsolidationProcess() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ms.consolidateMemories()
		case <-ms.ctx.Done():
			return
		}
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

// Memory decay process with graceful shutdown
func (ms *MemoryStore) startDecayProcess() {
	ticker := time.NewTicker(ms.decayInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ms.applyDecay()
		case <-ms.ctx.Done():
			return
		}
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

// Store a memory with comprehensive validation
func (mcp *MCPServer) StoreMemory(ctx context.Context, args StoreMemoryArgs) (*Memory, error) {
	// Validate arguments
	if args.Content == "" {
		return nil, errors.New("content cannot be empty")
	}
	if args.Type == "" {
		args.Type = ShortTerm // Default to short term
	}
	if args.Importance < 0 || args.Importance > 1 {
		args.Importance = 0.5 // Default importance
	}

	// Validate memory type
	validTypes := []MemoryType{ShortTerm, LongTerm, Episodic, Semantic, Procedural}
	valid := false
	for _, validType := range validTypes {
		if args.Type == validType {
			valid = true
			break
		}
	}
	if !valid {
		return nil, fmt.Errorf("invalid memory type: %s", args.Type)
	}

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

// Create relation between memories with validation
func (mcp *MCPServer) CreateRelation(ctx context.Context, args CreateRelationArgs) error {
	// Validate arguments
	if args.FromID == "" {
		return errors.New("from_id cannot be empty")
	}
	if args.ToID == "" {
		return errors.New("to_id cannot be empty")
	}
	if args.RelationType == "" {
		return errors.New("relation_type cannot be empty")
	}
	if args.Strength < 0 || args.Strength > 1 {
		args.Strength = 0.5 // Default strength
	}

	mcp.store.mu.Lock()
	defer mcp.store.mu.Unlock()

	// Check that both memories exist
	if _, exists := mcp.store.memories[args.FromID]; !exists {
		return fmt.Errorf("memory with ID %s does not exist", args.FromID)
	}
	if _, exists := mcp.store.memories[args.ToID]; !exists {
		return fmt.Errorf("memory with ID %s does not exist", args.ToID)
	}

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

// addToKeywordIndex indexes a memory by its content words
func (ms *MemoryStore) addToKeywordIndex(memory *Memory) {
	ms.keywordIndex.mu.Lock()
	defer ms.keywordIndex.mu.Unlock()

	// Extract and normalize words
	words := extractWords(memory.Content)
	for _, word := range words {
		if len(word) >= 3 { // Only index words with 3+ characters
			lowerWord := strings.ToLower(word)
			if ms.keywordIndex.index[lowerWord] == nil {
				ms.keywordIndex.index[lowerWord] = make(map[string]*Memory)
			}
			ms.keywordIndex.index[lowerWord][memory.ID] = memory
		}
	}
}

// removeFromKeywordIndex removes a memory from keyword index
func (ms *MemoryStore) removeFromKeywordIndex(memory *Memory) {
	ms.keywordIndex.mu.Lock()
	defer ms.keywordIndex.mu.Unlock()

	words := extractWords(memory.Content)
	for _, word := range words {
		if len(word) >= 3 {
			lowerWord := strings.ToLower(word)
			if memories, exists := ms.keywordIndex.index[lowerWord]; exists {
				delete(memories, memory.ID)
				// Clean up empty entries
				if len(memories) == 0 {
					delete(ms.keywordIndex.index, lowerWord)
				}
			}
		}
	}
}

// extractWords splits text into indexable words
func extractWords(text string) []string {
	// Simple word extraction - split on whitespace and punctuation
	words := strings.FieldsFunc(text, func(c rune) bool {
		return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))
	})
	return words
}

// normalizeVector normalizes a vector to unit length
func normalizeVector(v []float32) []float32 {
	var norm float32
	for _, val := range v {
		norm += val * val
	}
	norm = sqrt(norm)

	if norm == 0 {
		return v
	}

	normalized := make([]float32, len(v))
	for i, val := range v {
		normalized[i] = val / norm
	}
	return normalized
}

// dotProduct computes dot product of two vectors
func dotProduct(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

// cleanupTimeBuckets removes old time buckets to prevent memory leaks
func (ms *MemoryStore) cleanupTimeBuckets() {
	ms.timeIndex.mu.Lock()
	defer ms.timeIndex.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour * 7) // Keep only last 7 days
	cutoffStr := cutoff.Format("2006-01-02-15")

	for bucket := range ms.timeIndex.buckets {
		if bucket < cutoffStr {
			delete(ms.timeIndex.buckets, bucket)
		}
	}
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
