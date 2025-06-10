# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common Development Commands

```bash
# Build the server
go build -o mcp-memory-server .

# Build optimized version (smaller binary)
go build -ldflags="-s -w" -o mcp-memory-server .

# Format code
go fmt ./...

# Tidy dependencies
go mod tidy

# Run tests
go test -v

# Run tests with coverage
go test -cover

# Run benchmarks
go test -bench=. -benchmem

# Run the server (standalone mode)
./mcp-memory-server

# Run with custom settings
./mcp-memory-server --max-memories 1000 --max-memory-mb 50

# Run with memory sharing enabled
./mcp-memory-server --enable-sharing
```

## Architecture Overview

This is an MCP (Model Context Protocol) server that implements a cognitive-inspired memory system. The architecture consists of four main components:

### 1. MCP Protocol Layer (`mcp_server.go`)
- Implements MCP 2024-11-05 protocol over stdio transport
- Handles tool registration and invocation (store_memory, query_memories, create_relation, get_stats, wiki)
- Routes MCP messages to appropriate handlers
- Manages resources for memory statistics and graph visualization
- **Documentation Module** (`internal/docs/wiki.go`): Separated wiki documentation for better maintainability

### 2. Memory Store Core (`memory_store.go`) - RECENTLY OPTIMIZED
- **In-memory storage** with configurable limits (default 1000 memories, 100MB)
- **Memory types**: short_term, long_term, episodic, semantic, procedural
- **Automatic processes**:
  - Consolidation: Promotes frequently accessed short_term memories to long_term
  - Decay: Reduces importance over time, removes low-importance memories
  - Cleanup: Automatic time bucket cleanup prevents memory leaks
- **Optimized indexing strategies**:
  - **Keyword index**: Inverted index for O(1) text search (100x faster)
  - Type index for fast type-based queries
  - Time index using hour buckets with automatic cleanup
  - **Embedding index**: Pre-normalized vectors for faster similarity search
  - Relation graph for connected memory traversal
- **Performance optimizations**:
  - **Heap-based similarity search**: Top-K selection without full sorting
  - **Vector normalization**: Pre-computed for faster cosine similarity
  - **Swiss Tables**: Benefits from Go 1.24's faster map implementation

### 3. Configuration (`config.go`)
- Command-line flag parsing for runtime configuration
- Memory limits enforcement using Go 1.24's SetMemoryLimit
- GC tuning for low memory footprint
- Enable sharing flag for multi-client support

### 4. Connection Manager (`connection_manager.go`) - NEW
- Named pipe creation at `/tmp/mcp-memory-server.pipe`
- Auto-detection of running server instances
- Client connection forwarding when server exists
- Handoff protocol for client identification
- Multi-client connection management

## Key Design Patterns

1. **Concurrent Access**: Uses RWMutex for thread-safe operations on all indexes
2. **Memory Management**: Automatic eviction of least important memories when capacity reached
3. **Graph Relationships**: Memories can be linked with typed, weighted relationships
4. **Access Pattern Tracking**: Updates access count and last access time for better consolidation decisions
5. **Sub-millisecond Operations**: All operations optimized for in-memory performance
6. **Graceful Shutdown**: Context-based cancellation for clean resource cleanup
7. **Comprehensive Validation**: Input validation and error handling for all operations

## Recent Performance Improvements

### Keyword Search Optimization
- **Before**: O(n*m) linear search through all memories
- **After**: O(1) lookup using inverted index
- **Result**: 100x performance improvement

### Similarity Search Optimization  
- **Before**: O(n log n) full sort of all similarities
- **After**: O(n log k) heap-based top-K selection
- **Result**: 5-10x performance improvement for typical queries

### Memory Management
- **Time Bucket Cleanup**: Prevents unbounded growth of time indexes
- **Vector Normalization**: Pre-computed for faster similarity calculations
- **Enhanced Validation**: Comprehensive input validation and error handling

### Code Organization
- **Documentation Extraction**: Wiki documentation moved to `internal/docs/wiki.go` for better separation of concerns
- **Maintainability**: Large embedded strings extracted from server code for easier maintenance

## MCP Integration Points

The server exposes 5 MCP tools:
- `store_memory`: Creates new memories with cognitive type and metadata
- `query_memories`: Flexible query by similarity, keywords, type, time, or relationships
- `create_relation`: Links memories in a directed graph structure
- `get_stats`: Returns store statistics and capacity usage
- `wiki`: Provides comprehensive usage documentation

## Performance Considerations

### Core Optimizations
- **Keyword indexing**: 100x faster search with O(1) lookup
- **Heap-based similarity**: 5-10x faster top-K selection
- **Memory leak prevention**: Automatic cleanup of old time buckets
- **Vector optimization**: Pre-normalized embeddings for faster similarity

### Go 1.24 Runtime Benefits
- Swiss Tables implementation speeds up map operations significantly
- 2-3% CPU overhead reduction from runtime improvements
- Better small object allocation for reduced memory fragmentation
- Enhanced mutex performance for concurrent operations

### Resource Management
- Configurable memory limits prevent excessive resource usage
- Background goroutines with graceful shutdown via context cancellation
- Limited to 2 CPU cores via GOMAXPROCS for resource efficiency
- Automatic memory cleanup prevents unbounded growth

### Benchmark Results
- Keyword search: 2μs (100 memories) to 155μs (5000 memories)
- Similarity search: 35μs (100 embeddings) to 1.6ms (5000 embeddings)
- All operations remain sub-millisecond even with thousands of memories

## Memory Sharing Implementation

When `--enable-sharing` is used:
1. First client starts server and creates named pipe
2. Subsequent clients detect pipe and connect as clients
3. All stdio I/O is forwarded through the pipe
4. Single memory store instance shared by all clients
5. Automatic cleanup when last client disconnects

## Testing and Quality Assurance

The project includes a comprehensive test suite ensuring reliability and performance:

### Test Files
- **`memory_store_test.go`**: Core memory operations, indexing, concurrency
- **`mcp_server_test.go`**: MCP protocol handling, validation, error cases
- **`benchmark_test.go`**: Performance benchmarks and scaling tests

### Test Coverage
- **52.3% code coverage** across the project
- All critical paths and edge cases tested
- Concurrent access patterns verified
- Performance optimizations validated

### Running Tests
```bash
go test -v              # Run all tests
go test -cover          # Run with coverage
go test -bench=.        # Run performance benchmarks
go test -race           # Test for race conditions
```

## Task Management System

This project follows a structured task management approach for efficient feature development and project tracking.

### Project Management Framework

#### Strategic Planning (Top-Down)
- **`.project_management/potential_new_features.md`**: Strategic backlog of all potential features/projects under consideration
- **`.project_management/current_project.md`**: Active project scope with ordered feature pipeline in logical implementation order
- **`.project_management/current_feature.md`**: Current feature breakdown with specific tasks in execution order

#### Execution Flow (Bottom-Up)
- Start with `current_feature.md` and execute one task at a time
- Get user sign-off between tasks when they are completed and testable
- Strike through completed tasks and move to next
- Feature completion: strike feature from `current_project.md`, select next feature, update `current_feature.md`

### Task Guidelines

#### Task Granularity
- **Task Level**: Reasonable scope (e.g., "add database schema")
- **Sub-task Details**: Include sufficient detail within task description using bullet points
- **Philosophy**: Keep simple, avoid over-engineering, maintain iterative development flow

#### Feature Classification
- **`end-user-feature`**: Direct user-facing functionality
- **`tech-debt`**: Addressing existing code/architecture issues
- **`technical-improvements`**: Infrastructure, performance, developer experience

#### Feature Status Progression
- **`in-refinement`**: Discussion phase, scope unclear
- **`refined`**: Feature defined, tasks created, ready for work
- **`in-progress`**: First task started
- **`complete`**: Struck out in `current_project.md`

#### User Sign-off Process
- **Trigger**: When task(s) are completed and testable
- **Methods**: Demonstration (show what was built) OR user testing (hands-on validation)
- **Flexibility**: Context-dependent approach based on task type

## Documentation Management

This project maintains two documentation approaches with specific update strategies:

### Technical Documentation (`README.md`)
- **Target audience**: Developers, system administrators, technical users
- **Focus**: Architecture, setup guides, codebase essentials, technical specifications
- **Content strategy**: Concise but complete, just enough detail without becoming too large
- **Update timing**: Per feature completion (unless feature is too large, then per user request)
- **Value proposition**: Must provide clear value to technical audience

### Business Documentation (`business_readme.md`)
- **Target audience**: End users, business stakeholders, non-technical decision makers
- **Focus**: New features, behavior changes, use cases, practical examples
- **Content strategy**: User-friendly explanations with potential to evolve into user guide/release notes
- **Update timing**: Per feature completion (unless feature is too large, then per user request)
- **Value proposition**: Must provide clear value to non-technical end users

### Documentation Sync Requirements
When making changes that affect user-facing functionality:

1. **Update technical README** with implementation details, configuration options, and performance metrics
2. **Update business README** with new use cases, updated examples, and user-friendly explanations
3. **Maintain consistency** between both documents regarding feature capabilities
4. **Update CLAUDE.md** (this file) when architectural changes occur

**Important**: Both documentation files should be updated together when:
- New MCP tools are added
- Memory types or behaviors change  
- Performance characteristics improve significantly
- New use cases or workflows are enabled
- Installation or setup procedures change