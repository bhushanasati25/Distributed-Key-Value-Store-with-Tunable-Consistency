// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

const (
	baseURL1 = "http://localhost:8001"
	baseURL2 = "http://localhost:8002"
	baseURL3 = "http://localhost:8003"
)

// TestClusterBasicOperations tests basic CRUD operations on a cluster
func TestClusterBasicOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start cluster
	cleanup := startCluster(t)
	defer cleanup()

	time.Sleep(3 * time.Second) // Wait for nodes to start

	// Test PUT on node 1
	resp := httpPut(t, baseURL1+"/kv/testkey", `{"value":"hello-world"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test GET on node 2 (should be replicated)
	time.Sleep(500 * time.Millisecond)
	resp = httpGet(t, baseURL2+"/kv/testkey")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET failed with status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result["value"] != "hello-world" {
		t.Errorf("Expected 'hello-world', got '%v'", result["value"])
	}

	// Test GET on node 3
	resp = httpGet(t, baseURL3+"/kv/testkey")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET on node 3 failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestClusterNodeFailure tests behavior when a node fails
func TestClusterNodeFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cleanup := startCluster(t)
	defer cleanup()

	time.Sleep(3 * time.Second)

	// Write data
	resp := httpPut(t, baseURL1+"/kv/failtest", `{"value":"before-failure"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT failed with status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Kill node 2 (simulate failure)
	stopNode(t, 2)
	time.Sleep(2 * time.Second)

	// Should still be able to read from node 1 and 3
	resp = httpGet(t, baseURL1+"/kv/failtest")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET after failure should work with quorum")
	}
	resp.Body.Close()

	resp = httpGet(t, baseURL3+"/kv/failtest")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET from node 3 after failure should work")
	}
	resp.Body.Close()

	// Write should also work with quorum
	resp = httpPut(t, baseURL1+"/kv/afterfail", `{"value":"after-failure"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT after failure should work with quorum")
	}
	resp.Body.Close()
}

// TestClusterStatus tests the admin status endpoint
func TestClusterStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cleanup := startCluster(t)
	defer cleanup()

	time.Sleep(3 * time.Second)

	resp := httpGet(t, baseURL1+"/admin/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status endpoint failed with status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var status map[string]interface{}
	json.Unmarshal(body, &status)

	if _, ok := status["node_id"]; !ok {
		t.Error("Status should contain node_id")
	}
	if _, ok := status["uptime"]; !ok {
		t.Error("Status should contain uptime")
	}
}

// TestClusterManyKeys tests handling many keys
func TestClusterManyKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cleanup := startCluster(t)
	defer cleanup()

	time.Sleep(3 * time.Second)

	// Write 100 keys
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%03d", i)
		value := fmt.Sprintf("value-%03d", i)
		resp := httpPut(t, baseURL1+"/kv/"+key, fmt.Sprintf(`{"value":"%s"}`, value))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT %s failed with status %d", key, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Read from different nodes
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%03d", i)
		expectedValue := fmt.Sprintf("value-%03d", i)
		
		// Alternate between nodes
		url := baseURL1
		if i%3 == 1 {
			url = baseURL2
		} else if i%3 == 2 {
			url = baseURL3
		}
		
		resp := httpGet(t, url+"/kv/"+key)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s failed with status %d", key, resp.StatusCode)
			continue
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if result["value"] != expectedValue {
			t.Errorf("Key %s: expected '%s', got '%v'", key, expectedValue, result["value"])
		}
	}
}

// Helper functions

func startCluster(t *testing.T) func() {
	// Create temp directories
	dirs := []string{
		"/tmp/dynamo-test/node1",
		"/tmp/dynamo-test/node2",
		"/tmp/dynamo-test/node3",
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0755)
	}

	// Start nodes (these commands would be adjusted based on actual build)
	cmds := []*exec.Cmd{
		exec.Command("go", "run", "../../cmd/dynamo/main.go",
			"--node-id=node1", "--port=8001", "--gossip-port=7001", "--data-dir=/tmp/dynamo-test/node1"),
		exec.Command("go", "run", "../../cmd/dynamo/main.go",
			"--node-id=node2", "--port=8002", "--gossip-port=7002", "--data-dir=/tmp/dynamo-test/node2",
			"--seeds=127.0.0.1:7001"),
		exec.Command("go", "run", "../../cmd/dynamo/main.go",
			"--node-id=node3", "--port=8003", "--gossip-port=7003", "--data-dir=/tmp/dynamo-test/node3",
			"--seeds=127.0.0.1:7001,127.0.0.1:7002"),
	}

	for _, cmd := range cmds {
		cmd.Start()
	}

	return func() {
		for _, cmd := range cmds {
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
		}
		os.RemoveAll("/tmp/dynamo-test")
	}
}

func stopNode(t *testing.T, nodeNum int) {
	// Kill process on specific port
	cmd := exec.Command("lsof", "-t", fmt.Sprintf("-i:%d", 8000+nodeNum))
	output, _ := cmd.Output()
	if len(output) > 0 {
		exec.Command("kill", "-9", string(bytes.TrimSpace(output))).Run()
	}
}

func httpPut(t *testing.T, url string, body string) *http.Response {
	req, _ := http.NewRequest("PUT", url, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP PUT failed: %v", err)
	}
	return resp
}

func httpGet(t *testing.T, url string) *http.Response {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	return resp
}
