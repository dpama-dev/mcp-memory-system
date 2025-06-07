# Memory MCP Server

An in-memory AI memory system implemented as an MCP (Model Context Protocol) server. This server provides fast, cognitive-inspired memory storage and retrieval for AI agents without requiring external databases.

## Features

- **In-memory storage** - No external dependencies (Redis, PostgreSQL, etc.)
- **Sub-millisecond performance** - All operations happen in memory
- **Cognitive memory types** - Short-term, long-term, episodic, semantic, and procedural
- **Memory management** - Automatic decay, consolidation, and importance-based eviction
- **Relationship graphs** - Connect memories with typed relationships
- **Similarity search** - Find related memories using embeddings
- **Low resource usage** - Configurable memory limits (50-100MB)
- **Go 1.24 optimized** - Benefits from Swiss Tables and improved memory allocation

## Prerequisites

- macOS (Intel or Apple Silicon)
- Homebrew (for easy Go installation)
- Go 1.24 or later (benefits from performance improvements)

## Go Installation

1. Install Homebrew (if not already installed):
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

2. Install Go:
```bash
brew install go
```

3. Verify installation:
```bash
go version
# Should show: go version go1.24.x darwin/arm64 (or darwin/amd64)
```

4. Set up Go environment:
```bash
echo 'export GOPATH=$HOME/go' >> ~/.zshrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.zshrc
source ~/.zshrc
```

## Build Instructions

1. Clone the git repository

2. Navigate to project directory:
```bash
cd ~/go/src/mcp-memory-system
```

3. Build the server:
```bash
go build -o mcp-memory-server .
```

4. For optimized build (smaller binary):
```bash
go build -ldflags="-s -w" -o mcp-memory-server .
```

### Optional: Using Go 1.24 Tool Management

Go 1.24 introduces a new `tool` directive for managing development tools:

```bash
# Add development tools (optional)
go get -tool golang.org/x/tools/cmd/goimports@latest
go get -tool github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run tools
go tool goimports -w .
go tool golangci-lint run
```

## Running the Server

### Direct execution:
```bash
./mcp-memory-server
```

### With memory limits:
```bash
./mcp-memory-server --max-memories 1000 --max-memory-mb 100
```

### Available flags:
- `--max-memories`: Maximum number of memories to store (default: 1000)
- `--max-memory-mb`: Maximum memory usage in MB (default: 100)
- `--decay-interval`: Memory decay check interval (default: 5m)
- `--profile`: Enable memory profiling (default: false)


## MCP Client Configuration

### For Claude Desktop:

1. Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "memory": {
      "command": "/Users/YOUR_USERNAME/go/src/mcp-memory-system/mcp-memory-server",
      "args": ["--max-memories", "1000", "--max-memory-mb", "50"]
    }
  }
}
```

2. Restart Claude Desktop

### For Cursor:

1. Create or edit `~/.cursor/mcp.json`:
```json
{
  "mcpServers": {
    "memory": {
      "command": "/Users/YOUR_USERNAME/go/src/mcp-memory-system/mcp-memory-server",
      "args": ["--max-memories", "1000", "--max-memory-mb", "50"]
    }
  }
}
```

2. Restart Cursor

## Stopping the Server

### If running in terminal:
Press `Ctrl+C`

### If running in background:
```bash
pkill mcp-memory-server
```

### Check if server is running:
```bash
ps aux | grep mcp-memory-server
```

## Memory Monitoring

```bash
# Check memory usage
ps aux | grep mcp-memory-server

# Monitor in real-time
top -pid $(pgrep mcp-memory-server)

# View logs (if using LaunchAgent or service script)
tail -f ~/Library/Logs/mcp-memory-server.log
```

## Memory Sharing Between Clients (NEW)

You can now share memories between Claude Desktop and Claude Code! Use the `--enable-sharing` flag:

```bash
./mcp-memory-server --enable-sharing
```

This allows:
- First client starts the server
- Additional clients automatically connect to the existing server
- All clients share the same memory space
- Perfect for maintaining context across different Claude interfaces

## Available MCP Tools

1. **store_memory** - Store a new memory with type, content, and metadata
2. **query_memories** - Query memories by similarity, keywords, type, or relationships
3. **create_relation** - Create relationships between memories
4. **get_stats** - Get memory store statistics
5. **wiki** - Get comprehensive documentation on how to use the memory system

## Memory Types

- **short_term**: Temporary memories that may be consolidated
- **long_term**: Important memories that persist
- **episodic**: Event-based memories with temporal context
- **semantic**: Factual knowledge and concepts
- **procedural**: How-to knowledge and skills

## Performance

With Go 1.24, the memory server benefits from:
- **2-3% CPU overhead reduction** from runtime improvements
- **Faster map operations** using Swiss Tables implementation
- **More efficient memory allocation** for small objects
- **Better mutex performance** in concurrent operations

Typical operation latencies:
- Store Memory: <100μs
- Query Similar: <1ms
- Graph Traversal: <500μs
- Memory Update: <50μs

## Compatibility Notes

- **Go 1.24+**: This project is optimized for Go 1.24 and benefits from:
  - Swiss Tables-based map implementation (faster lookups)
  - Improved small object allocation (reduced memory overhead)
  - Better runtime mutex performance
- **Backwards Compatible**: Works with Go 1.21+ but best performance with Go 1.24+