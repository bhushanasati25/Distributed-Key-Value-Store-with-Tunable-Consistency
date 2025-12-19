package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// Request/Response types
type putRequest struct {
	Value       string `json:"value"`
	Consistency string `json:"consistency,omitempty"`
}

type getResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type errorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

type statusResponse struct {
	NodeID    string            `json:"node_id"`
	Address   string            `json:"address"`
	Uptime    string            `json:"uptime"`
	Keys      int64             `json:"keys"`
	Storage   storageStats      `json:"storage"`
	Cluster   clusterInfo       `json:"cluster,omitempty"`
}

type storageStats struct {
	ActiveKeys   int64  `json:"active_keys"`
	DeletedKeys  int64  `json:"deleted_keys"`
	DataFileSize int64  `json:"data_file_size_bytes"`
	TotalReads   uint64 `json:"total_reads"`
	TotalWrites  uint64 `json:"total_writes"`
}

type clusterInfo struct {
	Size  int        `json:"size"`
	Nodes []nodeInfo `json:"nodes"`
}

type nodeInfo struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	State   string `json:"state"`
}

// handleHealth returns the health status of the node
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"node":   s.config.NodeID,
	})
}

// handleGet retrieves a value by key
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	// Get consistency level from query param
	consistency := r.URL.Query().Get("consistency")
	if consistency == "" {
		consistency = "quorum"
	}

	// If we have a coordinator, use distributed read
	if s.coordinator != nil {
		value, version, err := s.coordinator.Get(r.Context(), key, types.ConsistencyLevel(consistency))
		if err != nil {
			if err.Error() == "key not found" {
				writeError(w, http.StatusNotFound, "key not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(getResponse{
			Key:     key,
			Value:   string(value),
			Version: version,
		})
		return
	}

	// Fallback to local storage
	value, timestamp, err := s.storage.Get(key)
	if err != nil {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(getResponse{
		Key:     key,
		Value:   string(value),
		Version: timestamp,
	})
}

// handlePut stores a key-value pair
func (s *Server) handlePut(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req putRequest
	if err := json.Unmarshal(body, &req); err != nil {
		// If not JSON, treat the entire body as the value
		req.Value = string(body)
	}

	if req.Value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return
	}

	consistency := req.Consistency
	if consistency == "" {
		consistency = "quorum"
	}

	// If we have a coordinator, use distributed write
	if s.coordinator != nil {
		err := s.coordinator.Put(r.Context(), key, []byte(req.Value), types.ConsistencyLevel(consistency))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"key":    key,
		})
		return
	}

	// Fallback to local storage
	timestamp := time.Now().UnixNano()
	if err := s.storage.Put(key, []byte(req.Value), timestamp); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"key":     key,
		"version": timestamp,
	})
}

// handleDelete removes a key
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	key := vars["key"]

	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	timestamp := time.Now().UnixNano()
	if err := s.storage.Delete(key, timestamp); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"key":    key,
	})
}

// handleStatus returns the node status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	stats := s.storage.Stats()

	response := statusResponse{
		NodeID:  s.config.NodeID,
		Address: s.config.FullAddress(),
		Uptime:  formatUptime(s.Uptime()),
		Keys:    stats.ActiveKeys,
		Storage: storageStats{
			ActiveKeys:   stats.ActiveKeys,
			DeletedKeys:  stats.DeletedKeys,
			DataFileSize: stats.DataFileSize,
			TotalReads:   stats.TotalReads,
			TotalWrites:  stats.TotalWrites,
		},
	}

	// Add cluster info if coordinator is available
	if s.coordinator != nil {
		nodes := s.coordinator.GetClusterNodes()
		clusterNodes := make([]nodeInfo, len(nodes))
		for i, n := range nodes {
			clusterNodes[i] = nodeInfo{
				ID:      n.ID,
				Address: n.Address,
				State:   n.State.String(),
			}
		}
		response.Cluster = clusterInfo{
			Size:  len(nodes),
			Nodes: clusterNodes,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRing returns the hash ring information
func (s *Server) handleRing(w http.ResponseWriter, r *http.Request) {
	if s.coordinator == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster mode not enabled")
		return
	}

	tokens := s.coordinator.GetRingTokens()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tokens": tokens,
		"count":  len(tokens),
	})
}

// handleKeys returns all keys (for debugging)
func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request) {
	keys := s.storage.Keys()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"keys":  keys,
		"count": len(keys),
	})
}

// handleStats returns detailed storage statistics
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.storage.Stats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleReplication handles internal replication requests
func (s *Server) handleReplication(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var req types.ReplicationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request format")
		return
	}

	// Store locally
	if req.Entry.IsDeleted {
		if err := s.storage.Delete(req.Entry.Key, req.Entry.Timestamp); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if err := s.storage.Put(req.Entry.Key, req.Entry.Value, req.Entry.Timestamp); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.ReplicationResponse{
		Success: true,
	})
}

// handleInternalRead handles internal read requests from other nodes
func (s *Server) handleInternalRead(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	value, timestamp, err := s.storage.Get(key)
	if err != nil {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.KeyValueEntry{
		Key:       key,
		Value:     value,
		Timestamp: timestamp,
	})
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errorResponse{
		Error:   http.StatusText(statusCode),
		Code:    statusCode,
		Message: message,
	})
}
