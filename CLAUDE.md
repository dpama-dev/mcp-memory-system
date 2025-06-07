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

# Run the server (standalone mode)
./mcp-memory-server

# Run with custom settings
./mcp-memory-server --max-memories 1000 --max-memory-mb 50

# Run with memory sharing enabled (NEW)
./mcp-memory-server --enable-sharing
```

## Architecture Overview

This is an MCP (Model Context Protocol) server that implements a cognitive-inspired memory system. The architecture consists of four main components:

### 1. MCP Protocol Layer (`mcp_server.go`)
- Implements MCP 2024-11-05 protocol over stdio transport
- Handles tool registration and invocation (store_memory, query_memories, create_relation, get_stats, wiki)
- Routes MCP messages to appropriate handlers
- Manages resources for memory statistics and graph visualization

### 2. Memory Store Core (`memory_store.go`)
- **In-memory storage** with configurable limits (default 1000 memories, 100MB)
- **Memory types**: short_term, long_term, episodic, semantic, procedural
- **Automatic processes**:
  - Consolidation: Promotes frequently accessed short_term memories to long_term
  - Decay: Reduces importance over time, removes low-importance memories
- **Indexing strategies**:
  - Type index for fast type-based queries
  - Time index using hour buckets for temporal queries
  - Embedding index for similarity search (384-dimensional)
  - Relation graph for connected memory traversal
- **Swiss Tables optimization**: Benefits from Go 1.24's faster map implementation

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

## MCP Integration Points

The server exposes 5 MCP tools:
- `store_memory`: Creates new memories with cognitive type and metadata
- `query_memories`: Flexible query by similarity, keywords, type, time, or relationships
- `create_relation`: Links memories in a directed graph structure
- `get_stats`: Returns store statistics and capacity usage
- `wiki`: Provides comprehensive usage documentation

## Performance Considerations

- Go 1.24 optimizations provide 2-3% CPU overhead reduction
- Swiss Tables implementation speeds up map operations
- Configurable memory limits prevent excessive resource usage
- Background goroutines for decay and consolidation run on separate timers
- Limited to 2 CPU cores via GOMAXPROCS for resource efficiency

## Memory Sharing Implementation

When `--enable-sharing` is used:
1. First client starts server and creates named pipe
2. Subsequent clients detect pipe and connect as clients
3. All stdio I/O is forwarded through the pipe
4. Single memory store instance shared by all clients
5. Automatic cleanup when last client disconnects