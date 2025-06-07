package main

import (
	"container/heap"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"
)

// Test basic memory store creation and initialization
func TestNewMemoryStore(t *testing.T) {
	store := NewMemoryStore(100)
	
	if store == nil {
		t.Fatal("NewMemoryStore returned nil")
	}
	
	if store.maxMemories != 100 {
		t.Errorf("Expected maxMemories to be 100, got %d", store.maxMemories)
	}
	
	if store.memories == nil {
		t.Error("memories map not initialized")
	}
	
	if store.keywordIndex == nil || store.keywordIndex.index == nil {
		t.Error("keyword index not initialized")
	}
	
	// Clean up
	store.Shutdown()
}

// Test storing and retrieving memories
func TestStoreAndRetrieve(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	// Test storing a valid memory
	memory := &Memory{
		ID:          "test-1",
		Type:        ShortTerm,
		Content:     "This is a test memory",
		Importance:  0.5,
		Timestamp:   time.Now(),
		LastAccess:  time.Now(),
		AccessCount: 0,
		Decay:       0.01,
	}
	
	err := store.Store(memory)
	if err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}
	
	// Verify memory was stored
	store.mu.RLock()
	stored, exists := store.memories[memory.ID]
	store.mu.RUnlock()
	
	if !exists {
		t.Error("Memory not found in store")
	}
	
	if stored.Content != memory.Content {
		t.Errorf("Expected content %s, got %s", memory.Content, stored.Content)
	}
}

// Test validation errors
func TestStoreValidation(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	tests := []struct {
		name    string
		memory  *Memory
		wantErr bool
	}{
		{
			name:    "nil memory",
			memory:  nil,
			wantErr: true,
		},
		{
			name: "empty ID",
			memory: &Memory{
				ID:         "",
				Type:       ShortTerm,
				Content:    "test",
				Importance: 0.5,
			},
			wantErr: true,
		},
		{
			name: "empty content",
			memory: &Memory{
				ID:         "test-1",
				Type:       ShortTerm,
				Content:    "",
				Importance: 0.5,
			},
			wantErr: true,
		},
		{
			name: "invalid importance low",
			memory: &Memory{
				ID:         "test-1",
				Type:       ShortTerm,
				Content:    "test",
				Importance: -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid importance high",
			memory: &Memory{
				ID:         "test-1",
				Type:       ShortTerm,
				Content:    "test",
				Importance: 1.1,
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Store(tt.memory)
			if (err != nil) != tt.wantErr {
				t.Errorf("Store() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test duplicate ID handling
func TestStoreDuplicateID(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	memory1 := &Memory{
		ID:         "dup-1",
		Type:       ShortTerm,
		Content:    "First memory",
		Importance: 0.5,
	}
	
	memory2 := &Memory{
		ID:         "dup-1", // Same ID
		Type:       LongTerm,
		Content:    "Second memory",
		Importance: 0.7,
	}
	
	// First store should succeed
	err := store.Store(memory1)
	if err != nil {
		t.Fatalf("First store failed: %v", err)
	}
	
	// Second store should fail
	err = store.Store(memory2)
	if err == nil {
		t.Error("Expected error for duplicate ID, got nil")
	}
}

// Test memory eviction when at capacity
func TestMemoryEviction(t *testing.T) {
	store := NewMemoryStore(3) // Small capacity
	defer store.Shutdown()
	
	// Store memories with different importance levels
	memories := []*Memory{
		{ID: "low", Type: ShortTerm, Content: "Low importance", Importance: 0.1},
		{ID: "med", Type: ShortTerm, Content: "Medium importance", Importance: 0.5},
		{ID: "high", Type: ShortTerm, Content: "High importance", Importance: 0.9},
	}
	
	for _, m := range memories {
		if err := store.Store(m); err != nil {
			t.Fatalf("Failed to store memory %s: %v", m.ID, err)
		}
	}
	
	// Store one more memory - should evict lowest importance
	newMem := &Memory{
		ID:         "new",
		Type:       ShortTerm,
		Content:    "New memory",
		Importance: 0.6,
	}
	
	if err := store.Store(newMem); err != nil {
		t.Fatalf("Failed to store new memory: %v", err)
	}
	
	// Check that low importance memory was evicted
	store.mu.RLock()
	_, lowExists := store.memories["low"]
	_, newExists := store.memories["new"]
	store.mu.RUnlock()
	
	if lowExists {
		t.Error("Low importance memory should have been evicted")
	}
	
	if !newExists {
		t.Error("New memory should exist")
	}
}

// Test keyword indexing
func TestKeywordIndexing(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	memory := &Memory{
		ID:         "test-1",
		Type:       ShortTerm,
		Content:    "The quick brown fox jumps over the lazy dog",
		Importance: 0.5,
	}
	
	if err := store.Store(memory); err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}
	
	// Test keyword search
	results := store.findByKeywords([]string{"quick", "fox"})
	
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	
	if len(results) > 0 && results[0].ID != "test-1" {
		t.Errorf("Expected memory ID test-1, got %s", results[0].ID)
	}
	
	// Test non-existent keyword
	results = store.findByKeywords([]string{"elephant"})
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent keyword, got %d", len(results))
	}
}

// Test case-insensitive keyword search
func TestKeywordSearchCaseInsensitive(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	memory := &Memory{
		ID:         "test-1",
		Type:       ShortTerm,
		Content:    "The QUICK Brown FOX",
		Importance: 0.5,
	}
	
	if err := store.Store(memory); err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}
	
	// Test with different cases
	testCases := []string{"quick", "QUICK", "Quick", "fox", "FOX", "Fox"}
	
	for _, keyword := range testCases {
		results := store.findByKeywords([]string{keyword})
		if len(results) != 1 {
			t.Errorf("Expected 1 result for keyword %s, got %d", keyword, len(results))
		}
	}
}

// Test memory removal and index cleanup
func TestMemoryRemoval(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	memory := &Memory{
		ID:         "test-remove",
		Type:       ShortTerm,
		Content:    "Memory to be removed with unique keyword xyzabc",
		Importance: 0.5,
	}
	
	if err := store.Store(memory); err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}
	
	// Verify memory exists in keyword index
	results := store.findByKeywords([]string{"xyzabc"})
	if len(results) != 1 {
		t.Error("Memory not found in keyword index")
	}
	
	// Remove memory
	store.mu.Lock()
	store.removeMemory("test-remove")
	store.mu.Unlock()
	
	// Verify memory is removed from all indexes
	store.mu.RLock()
	_, exists := store.memories["test-remove"]
	store.mu.RUnlock()
	
	if exists {
		t.Error("Memory still exists in main store")
	}
	
	// Check keyword index
	results = store.findByKeywords([]string{"xyzabc"})
	if len(results) != 0 {
		t.Error("Memory still exists in keyword index after removal")
	}
}

// Test query validation
func TestQueryValidation(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	tests := []struct {
		name     string
		criteria QueryCriteria
		wantErr  bool
	}{
		{
			name:     "empty query type",
			criteria: QueryCriteria{Type: ""},
			wantErr:  true,
		},
		{
			name:     "negative limit",
			criteria: QueryCriteria{Type: "keywords", Limit: -1},
			wantErr:  true,
		},
		{
			name:     "limit too high",
			criteria: QueryCriteria{Type: "keywords", Limit: 1001},
			wantErr:  true,
		},
		{
			name:     "valid query",
			criteria: QueryCriteria{Type: "keywords", Keywords: []string{"test"}, Limit: 10},
			wantErr:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Query(tt.criteria)
			if (err != nil) != tt.wantErr {
				t.Errorf("Query() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test concurrent access
func TestConcurrentAccess(t *testing.T) {
	store := NewMemoryStore(100)
	defer store.Shutdown()
	
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 10
	
	// Concurrent writes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				memory := &Memory{
					ID:         fmt.Sprintf("g%d-m%d", goroutineID, j),
					Type:       ShortTerm,
					Content:    fmt.Sprintf("Memory from goroutine %d, operation %d", goroutineID, j),
					Importance: 0.5,
				}
				if err := store.Store(memory); err != nil {
					t.Errorf("Concurrent store failed: %v", err)
				}
			}
		}(i)
	}
	
	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				criteria := QueryCriteria{
					Type:     "keywords",
					Keywords: []string{"memory"},
					Limit:    10,
				}
				if _, err := store.Query(criteria); err != nil {
					t.Errorf("Concurrent query failed: %v", err)
				}
			}
		}()
	}
	
	wg.Wait()
	
	// Verify all memories were stored
	store.mu.RLock()
	memCount := len(store.memories)
	store.mu.RUnlock()
	
	expectedCount := numGoroutines * numOperations
	if memCount != expectedCount {
		t.Errorf("Expected %d memories, got %d", expectedCount, memCount)
	}
}

// Test time-based indexing
func TestTimeIndexing(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	now := time.Now()
	
	// Store memories at different times
	memories := []*Memory{
		{
			ID:         "past",
			Type:       ShortTerm,
			Content:    "Past memory",
			Importance: 0.5,
			Timestamp:  now.Add(-2 * time.Hour),
		},
		{
			ID:         "recent",
			Type:       ShortTerm,
			Content:    "Recent memory",
			Importance: 0.5,
			Timestamp:  now.Add(-30 * time.Minute),
		},
		{
			ID:         "current",
			Type:       ShortTerm,
			Content:    "Current memory",
			Importance: 0.5,
			Timestamp:  now,
		},
	}
	
	for _, m := range memories {
		m.LastAccess = m.Timestamp
		if err := store.Store(m); err != nil {
			t.Fatalf("Failed to store memory: %v", err)
		}
	}
	
	// Query temporal range
	results := store.findTemporal(now.Add(-1*time.Hour), now.Add(1*time.Hour))
	
	if len(results) != 2 {
		t.Errorf("Expected 2 memories in temporal range, got %d", len(results))
	}
}

// Test memory type filtering
func TestMemoryTypeFiltering(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	types := []MemoryType{ShortTerm, LongTerm, Episodic, Semantic, Procedural}
	
	// Store one memory of each type
	for i, memType := range types {
		memory := &Memory{
			ID:         fmt.Sprintf("type-%d", i),
			Type:       memType,
			Content:    fmt.Sprintf("Memory of type %s", memType),
			Importance: 0.5,
		}
		if err := store.Store(memory); err != nil {
			t.Fatalf("Failed to store memory: %v", err)
		}
	}
	
	// Query each type
	for _, memType := range types {
		results := store.findByType(memType)
		if len(results) != 1 {
			t.Errorf("Expected 1 memory of type %s, got %d", memType, len(results))
		}
		if len(results) > 0 && results[0].Type != memType {
			t.Errorf("Expected memory type %s, got %s", memType, results[0].Type)
		}
	}
}

// Test extractWords function
func TestExtractWords(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "Hello world",
			expected: []string{"Hello", "world"},
		},
		{
			input:    "Test123 with-punctuation!",
			expected: []string{"Test123", "with", "punctuation"},
		},
		{
			input:    "   Multiple   spaces   ",
			expected: []string{"Multiple", "spaces"},
		},
		{
			input:    "a1b2c3",
			expected: []string{"a1b2c3"},
		},
		{
			input:    "",
			expected: []string{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractWords(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("extractWords(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Test vector normalization
func TestNormalizeVector(t *testing.T) {
	tests := []struct {
		name     string
		input    []float32
		expected []float32
	}{
		{
			name:     "unit vector",
			input:    []float32{1, 0, 0},
			expected: []float32{1, 0, 0},
		},
		{
			name:     "non-unit vector",
			input:    []float32{3, 4, 0},
			expected: []float32{0.6, 0.8, 0},
		},
		{
			name:     "zero vector",
			input:    []float32{0, 0, 0},
			expected: []float32{0, 0, 0},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeVector(tt.input)
			for i := range result {
				if !floatEquals(result[i], tt.expected[i], 0.0001) {
					t.Errorf("normalizeVector() element %d = %v, want %v", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

// Helper function for float comparison
func floatEquals(a, b, epsilon float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// Test graceful shutdown
func TestGracefulShutdown(t *testing.T) {
	store := NewMemoryStore(10)
	
	// Store a memory
	memory := &Memory{
		ID:         "shutdown-test",
		Type:       ShortTerm,
		Content:    "Test memory",
		Importance: 0.5,
	}
	
	if err := store.Store(memory); err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}
	
	// Shutdown
	store.Shutdown()
	
	// Verify context is cancelled
	select {
	case <-store.ctx.Done():
		// Expected
	default:
		t.Error("Context not cancelled after shutdown")
	}
}

// Test memory access count updates
func TestAccessCountUpdate(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	memory := &Memory{
		ID:          "access-test",
		Type:        ShortTerm,
		Content:     "Test memory for access count",
		Importance:  0.5,
		AccessCount: 0,
	}
	
	if err := store.Store(memory); err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}
	
	// Query the memory multiple times
	for i := 0; i < 3; i++ {
		criteria := QueryCriteria{
			Type:     "keywords",
			Keywords: []string{"access"},
			Limit:    10,
		}
		results, err := store.Query(criteria)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		
		// Verify access count increased
		if results[0].AccessCount != i+1 {
			t.Errorf("Expected access count %d, got %d", i+1, results[0].AccessCount)
		}
	}
}

// Test heap-based similarity search
func TestSimilaritySearch(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	// Create test embeddings
	memories := []*Memory{
		{
			ID:         "sim-1",
			Type:       ShortTerm,
			Content:    "First memory",
			Embedding:  []float32{1, 0, 0},
			Importance: 0.5,
		},
		{
			ID:         "sim-2",
			Type:       ShortTerm,
			Content:    "Second memory",
			Embedding:  []float32{0.8, 0.6, 0},
			Importance: 0.5,
		},
		{
			ID:         "sim-3",
			Type:       ShortTerm,
			Content:    "Third memory",
			Embedding:  []float32{0, 1, 0},
			Importance: 0.5,
		},
	}
	
	for _, m := range memories {
		if err := store.Store(m); err != nil {
			t.Fatalf("Failed to store memory: %v", err)
		}
	}
	
	// Search for similar memories to [1, 0, 0]
	queryEmbedding := []float32{1, 0, 0}
	results := store.findSimilar(queryEmbedding, 2)
	
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	
	// First result should be sim-1 (exact match)
	if results[0].ID != "sim-1" {
		t.Errorf("Expected first result to be sim-1, got %s", results[0].ID)
	}
	
	// Second result should be sim-2 (most similar)
	if results[1].ID != "sim-2" {
		t.Errorf("Expected second result to be sim-2, got %s", results[1].ID)
	}
}

// Test ScoredMemoryHeap implementation
func TestScoredMemoryHeap(t *testing.T) {
	h := &ScoredMemoryHeap{}
	heap.Init(h)
	
	// Push items with different scores
	items := []*ScoredMemory{
		{Memory: &Memory{ID: "1"}, Score: 0.5},
		{Memory: &Memory{ID: "2"}, Score: 0.8},
		{Memory: &Memory{ID: "3"}, Score: 0.3},
		{Memory: &Memory{ID: "4"}, Score: 0.9},
		{Memory: &Memory{ID: "5"}, Score: 0.1},
	}
	
	for _, item := range items {
		heap.Push(h, item)
	}
	
	// Test that we get items in ascending order (min-heap)
	expectedOrder := []float32{0.1, 0.3, 0.5, 0.8, 0.9}
	for _, expectedScore := range expectedOrder {
		if h.Len() == 0 {
			t.Fatal("Heap empty too early")
		}
		
		item := heap.Pop(h).(*ScoredMemory)
		if !floatEquals(item.Score, expectedScore, 0.0001) {
			t.Errorf("Expected score %f, got %f", expectedScore, item.Score)
		}
	}
	
	if h.Len() != 0 {
		t.Errorf("Heap should be empty, but has %d items", h.Len())
	}
}

// Test top-K selection with heap
func TestTopKSelection(t *testing.T) {
	store := NewMemoryStore(20)
	defer store.Shutdown()
	
	// Create many memories with embeddings
	for i := 0; i < 20; i++ {
		angle := float64(i) * math.Pi / 10.0
		memory := &Memory{
			ID:         fmt.Sprintf("vec-%d", i),
			Type:       ShortTerm,
			Content:    fmt.Sprintf("Memory %d", i),
			Embedding:  []float32{float32(math.Cos(angle)), float32(math.Sin(angle)), 0},
			Importance: 0.5,
		}
		if err := store.Store(memory); err != nil {
			t.Fatalf("Failed to store memory: %v", err)
		}
	}
	
	// Query for top 5 similar to [1, 0, 0]
	queryEmbedding := []float32{1, 0, 0}
	results := store.findSimilar(queryEmbedding, 5)
	
	if len(results) != 5 {
		t.Fatalf("Expected 5 results, got %d", len(results))
	}
	
	// Verify results are in descending order of similarity
	for i := 1; i < len(results); i++ {
		// Calculate actual similarities
		prevSim := dotProduct(normalizeVector(queryEmbedding), 
			normalizeVector(results[i-1].Embedding))
		currSim := dotProduct(normalizeVector(queryEmbedding), 
			normalizeVector(results[i].Embedding))
		
		if currSim > prevSim {
			t.Errorf("Results not in descending order: %f > %f", currSim, prevSim)
		}
	}
}

// Test dot product calculation
func TestDotProduct(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0,
		},
		{
			name:     "parallel vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{2, 0, 0},
			expected: 2,
		},
		{
			name:     "general case",
			a:        []float32{1, 2, 3},
			b:        []float32{4, 5, 6},
			expected: 32, // 1*4 + 2*5 + 3*6
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dotProduct(tt.a, tt.b)
			if !floatEquals(result, tt.expected, 0.0001) {
				t.Errorf("dotProduct() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Test memory consolidation
func TestMemoryConsolidation(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	// Create a short-term memory with high access count
	memory := &Memory{
		ID:          "consolidate-test",
		Type:        ShortTerm,
		Content:     "Frequently accessed memory",
		Importance:  0.6,
		AccessCount: 5, // High access count
	}
	
	if err := store.Store(memory); err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}
	
	// Manually trigger consolidation
	store.consolidateMemories()
	
	// Check if memory was promoted to long-term
	store.mu.RLock()
	mem, exists := store.memories["consolidate-test"]
	store.mu.RUnlock()
	
	if !exists {
		t.Fatal("Memory disappeared after consolidation")
	}
	
	if mem.Type != LongTerm {
		t.Errorf("Expected memory type to be LongTerm after consolidation, got %s", mem.Type)
	}
}

// Test time bucket cleanup
func TestTimeBucketCleanup(t *testing.T) {
	store := NewMemoryStore(10)
	defer store.Shutdown()
	
	// Add old memories directly to time index
	oldTime := time.Now().Add(-10 * 24 * time.Hour) // 10 days ago
	oldBucket := oldTime.Format("2006-01-02-15")
	
	store.timeIndex.mu.Lock()
	store.timeIndex.buckets[oldBucket] = []*Memory{
		{ID: "old-memory", Content: "Old memory"},
	}
	store.timeIndex.mu.Unlock()
	
	// Run cleanup
	store.cleanupTimeBuckets()
	
	// Check that old bucket was removed
	store.timeIndex.mu.RLock()
	_, exists := store.timeIndex.buckets[oldBucket]
	store.timeIndex.mu.RUnlock()
	
	if exists {
		t.Error("Old time bucket should have been cleaned up")
	}
}