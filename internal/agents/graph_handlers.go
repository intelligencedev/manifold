// graph_handlers.go
package agents

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	configpkg "manifold/internal/config"
)

// FindMemoryPathHandler handles GET /api/memory/path/:sourceId/:targetId
func FindMemoryPathHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled"})
		}

		sourceIdStr := c.Param("sourceId")
		targetIdStr := c.Param("targetId")

		sourceId, err := strconv.ParseInt(sourceIdStr, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid source ID"})
		}

		targetId, err := strconv.ParseInt(targetIdStr, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid target ID"})
		}

		ctx := c.Request().Context()

		// Get database connection
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}

		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		// Use enhanced engine for graph operations
		engine := NewEnhancedAgenticEngine(conn.Conn())
		err = engine.EnsureEnhancedMemoryTables(ctx, config.Embeddings.Dimensions)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ensure enhanced tables"})
		}

		path, err := engine.FindMemoryPath(ctx, sourceId, targetId)
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{"path": path})
	}
}

// FindRelatedMemoriesHandler handles GET /api/memory/related/:memoryId
func FindRelatedMemoriesHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled"})
		}

		memoryIdStr := c.Param("memoryId")
		memoryId, err := strconv.ParseInt(memoryIdStr, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid memory ID"})
		}

		// Get query parameters
		hopsStr := c.QueryParam("hops")
		hops := 2 // default
		if hopsStr != "" {
			if h, err := strconv.Atoi(hopsStr); err == nil {
				hops = h
			}
		}

		relationshipTypes := c.QueryParams()["type"] // Allow multiple types

		ctx := c.Request().Context()

		// Get database connection
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}

		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		engine := NewEnhancedAgenticEngine(conn.Conn())
		err = engine.EnsureEnhancedMemoryTables(ctx, config.Embeddings.Dimensions)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ensure enhanced tables"})
		}

		relatedMemories, err := engine.FindRelatedMemories(ctx, memoryId, hops, relationshipTypes)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{"related_memories": relatedMemories})
	}
}

// DiscoverMemoryClustersHandler handles GET /api/memory/clusters/:workflowId
func DiscoverMemoryClustersHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled"})
		}

		workflowIdStr := c.Param("workflowId")
		workflowId, err := uuid.Parse(workflowIdStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid workflow ID"})
		}

		// Get query parameters
		minClusterSizeStr := c.QueryParam("min_size")
		minClusterSize := 3 // default
		if minClusterSizeStr != "" {
			if size, err := strconv.Atoi(minClusterSizeStr); err == nil {
				minClusterSize = size
			}
		}

		ctx := c.Request().Context()

		// Get database connection
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}

		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		engine := NewEnhancedAgenticEngine(conn.Conn())
		err = engine.EnsureEnhancedMemoryTables(ctx, config.Embeddings.Dimensions)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ensure enhanced tables"})
		}

		clusters, err := engine.DiscoverMemoryClusters(ctx, workflowId, minClusterSize)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{"clusters": clusters})
	}
}

// AnalyzeNetworkHealthHandler handles GET /api/memory/health/:workflowId
func AnalyzeNetworkHealthHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled"})
		}

		workflowIdStr := c.Param("workflowId")
		workflowId, err := uuid.Parse(workflowIdStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid workflow ID"})
		}

		ctx := c.Request().Context()

		// Get database connection
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}

		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		engine := NewEnhancedAgenticEngine(conn.Conn())
		err = engine.EnsureEnhancedMemoryTables(ctx, config.Embeddings.Dimensions)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ensure enhanced tables"})
		}

		health, err := engine.AnalyzeMemoryNetworkHealth(ctx, workflowId)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{"health": health})
	}
}

// BuildKnowledgeMapHandler handles GET /api/memory/knowledge-map/:workflowId
func BuildKnowledgeMapHandler(config *configpkg.Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !config.AgenticMemory.Enabled {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "Agentic memory is disabled"})
		}

		workflowIdStr := c.Param("workflowId")
		workflowId, err := uuid.Parse(workflowIdStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid workflow ID"})
		}

		// Get query parameters
		depthStr := c.QueryParam("depth")
		depth := 3 // default
		if depthStr != "" {
			if d, err := strconv.Atoi(depthStr); err == nil {
				depth = d
			}
		}

		ctx := c.Request().Context()

		// Get database connection
		if config.DBPool == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection pool not initialized"})
		}

		conn, err := config.DBPool.Acquire(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to acquire database connection"})
		}
		defer conn.Release()

		engine := NewEnhancedAgenticEngine(conn.Conn())
		err = engine.EnsureEnhancedMemoryTables(ctx, config.Embeddings.Dimensions)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to ensure enhanced tables"})
		}

		knowledgeMap, err := engine.BuildKnowledgeMap(ctx, workflowId, depth)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{"knowledge_map": knowledgeMap})
	}
}
