package main

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// Benchmark keyword search with indexing
func BenchmarkKeywordSearch(b *testing.B) {
	sizes := []int{100, 1000, 5000}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			store := NewMemoryStore(size + 100)
			defer store.Shutdown()
			
			// Populate store with memories
			words := []string{"apple", "banana", "cherry", "date", "elderberry", "fig", "grape", "honeydew"}
			for i := 0; i < size; i++ {
				// Create content with random words
				content := fmt.Sprintf("Memory %d contains %s and %s", 
					i, 
					words[rand.Intn(len(words))],
					words[rand.Intn(len(words))],
				)
				
				memory := &Memory{
					ID:         fmt.Sprintf("mem-%d", i),
					Type:       ShortTerm,
					Content:    content,
					Importance: 0.5,
				}
				store.Store(memory)
			}
			
			// Benchmark keyword search
			keywords := []string{"apple", "banana"}
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = store.findByKeywords(keywords)
			}
		})
	}
}

// Benchmark similarity search with heap
func BenchmarkSimilaritySearch(b *testing.B) {
	sizes := []int{100, 1000, 5000}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			store := NewMemoryStore(size + 100)
			defer store.Shutdown()
			
			// Populate with random embeddings
			for i := 0; i < size; i++ {
				embedding := make([]float32, 384)
				for j := range embedding {
					embedding[j] = rand.Float32()*2 - 1 // Random values between -1 and 1
				}
				
				memory := &Memory{
					ID:         fmt.Sprintf("vec-%d", i),
					Type:       ShortTerm,
					Content:    fmt.Sprintf("Memory %d", i),
					Embedding:  embedding,
					Importance: 0.5,
				}
				store.Store(memory)
			}
			
			// Create query embedding
			queryEmbedding := make([]float32, 384)
			for i := range queryEmbedding {
				queryEmbedding[i] = rand.Float32()*2 - 1
			}
			
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = store.findSimilar(queryEmbedding, 10)
			}
		})
	}
}

// Benchmark memory store operations
func BenchmarkMemoryStore(b *testing.B) {
	b.Run("Store", func(b *testing.B) {
		store := NewMemoryStore(10000)
		defer store.Shutdown()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			memory := &Memory{
				ID:         fmt.Sprintf("bench-%d", i),
				Type:       ShortTerm,
				Content:    fmt.Sprintf("Benchmark memory %d", i),
				Importance: 0.5,
			}
			store.Store(memory)
		}
	})
	
	b.Run("Query", func(b *testing.B) {
		store := NewMemoryStore(1000)
		defer store.Shutdown()
		
		// Pre-populate
		for i := 0; i < 1000; i++ {
			memory := &Memory{
				ID:         fmt.Sprintf("pre-%d", i),
				Type:       ShortTerm,
				Content:    fmt.Sprintf("Memory %d with keyword test", i),
				Importance: 0.5,
			}
			store.Store(memory)
		}
		
		criteria := QueryCriteria{
			Type:     "keywords",
			Keywords: []string{"test"},
			Limit:    10,
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			store.Query(criteria)
		}
	})
}

// Benchmark concurrent access
func BenchmarkConcurrentAccess(b *testing.B) {
	store := NewMemoryStore(10000)
	defer store.Shutdown()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		memory := &Memory{
			ID:         fmt.Sprintf("init-%d", i),
			Type:       ShortTerm,
			Content:    fmt.Sprintf("Initial memory %d", i),
			Importance: 0.5,
		}
		store.Store(memory)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				// Write operation
				memory := &Memory{
					ID:         fmt.Sprintf("concurrent-%d-%d", i, time.Now().UnixNano()),
					Type:       ShortTerm,
					Content:    "Concurrent memory",
					Importance: 0.5,
				}
				store.Store(memory)
			} else {
				// Read operation
				criteria := QueryCriteria{
					Type:  "type",
					MemoryType: ShortTerm,
					Limit: 5,
				}
				store.Query(criteria)
			}
			i++
		}
	})
}

// Benchmark vector operations
func BenchmarkVectorOperations(b *testing.B) {
	b.Run("Normalize", func(b *testing.B) {
		vectors := make([][]float32, 100)
		for i := range vectors {
			vec := make([]float32, 384)
			for j := range vec {
				vec[j] = rand.Float32()*2 - 1
			}
			vectors[i] = vec
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = normalizeVector(vectors[i%len(vectors)])
		}
	})
	
	b.Run("DotProduct", func(b *testing.B) {
		// Create normalized vectors
		vectors := make([][]float32, 100)
		for i := range vectors {
			vec := make([]float32, 384)
			for j := range vec {
				vec[j] = rand.Float32()*2 - 1
			}
			vectors[i] = normalizeVector(vec)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx1 := i % len(vectors)
			idx2 := (i + 1) % len(vectors)
			_ = dotProduct(vectors[idx1], vectors[idx2])
		}
	})
}

// Benchmark keyword extraction
func BenchmarkKeywordExtraction(b *testing.B) {
	texts := []string{
		"The quick brown fox jumps over the lazy dog",
		"Lorem ipsum dolor sit amet, consectetur adipiscing elit",
		"Go is an open source programming language that makes it easy to build simple, reliable, and efficient software",
		"Benchmark tests are important for validating performance improvements",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = extractWords(texts[i%len(texts)])
	}
}

// Benchmark time-based cleanup
func BenchmarkTimeBucketCleanup(b *testing.B) {
	store := NewMemoryStore(1000)
	defer store.Shutdown()
	
	// Create many time buckets
	now := time.Now()
	for i := 0; i < 500; i++ {
		bucketTime := now.Add(-time.Duration(i) * time.Hour)
		bucket := bucketTime.Format("2006-01-02-15")
		
		store.timeIndex.mu.Lock()
		store.timeIndex.buckets[bucket] = []*Memory{
			{ID: fmt.Sprintf("old-%d", i), Content: "Old memory"},
		}
		store.timeIndex.mu.Unlock()
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.cleanupTimeBuckets()
	}
}

// Benchmark MCP protocol handling
func BenchmarkMCPProtocol(b *testing.B) {
	store := NewMemoryStore(1000)
	defer store.Shutdown()
	server := &MCPServer{store: store}
	
	b.Run("StoreMemory", func(b *testing.B) {
		args := StoreMemoryArgs{
			Type:       ShortTerm,
			Content:    "Benchmark memory content",
			Importance: 0.5,
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = server.StoreMemory(nil, args)
		}
	})
	
	b.Run("QueryMemories", func(b *testing.B) {
		// Pre-populate
		for i := 0; i < 100; i++ {
			args := StoreMemoryArgs{
				Type:       ShortTerm,
				Content:    fmt.Sprintf("Memory %d with benchmark keyword", i),
				Importance: 0.5,
			}
			server.StoreMemory(nil, args)
		}
		
		queryArgs := QueryMemoryArgs{
			QueryType: "keywords",
			Keywords:  []string{"benchmark"},
			Limit:     10,
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = server.QueryMemories(nil, queryArgs)
		}
	})
}

// Benchmark memory eviction
func BenchmarkMemoryEviction(b *testing.B) {
	store := NewMemoryStore(100) // Small capacity to trigger eviction
	defer store.Shutdown()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memory := &Memory{
			ID:         fmt.Sprintf("evict-%d", i),
			Type:       ShortTerm,
			Content:    fmt.Sprintf("Memory %d", i),
			Importance: rand.Float32(), // Random importance
		}
		store.Store(memory)
	}
}