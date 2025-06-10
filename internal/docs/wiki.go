package docs

// GetWiki returns comprehensive documentation on how to use the memory system
func GetWiki() string {
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