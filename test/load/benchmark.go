package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var (
	target      = flag.String("target", "http://localhost:8001", "Target node URL")
	requests    = flag.Int("requests", 1000, "Total number of requests")
	concurrency = flag.Int("concurrency", 10, "Number of concurrent workers")
	ratio       = flag.Float64("write-ratio", 0.5, "Ratio of write operations (0-1)")
	keySpace    = flag.Int("key-space", 1000, "Number of unique keys")
)

type Stats struct {
	totalRequests  int64
	successfulReqs int64
	failedReqs     int64
	totalLatency   int64 // in microseconds
	minLatency     int64
	maxLatency     int64
}

func main() {
	flag.Parse()

	fmt.Printf("Distributed Key-Value Store Load Tester\n")
	fmt.Printf("=======================\n")
	fmt.Printf("Target: %s\n", *target)
	fmt.Printf("Requests: %d\n", *requests)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	fmt.Printf("Write Ratio: %.1f%%\n", *ratio*100)
	fmt.Printf("Key Space: %d keys\n\n", *keySpace)

	stats := &Stats{
		minLatency: 999999999,
	}

	// Create work channel
	work := make(chan int, *requests)
	for i := 0; i < *requests; i++ {
		work <- i
	}
	close(work)

	// Start workers
	var wg sync.WaitGroup
	startTime := time.Now()

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go worker(work, stats, &wg)
	}

	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				completed := atomic.LoadInt64(&stats.totalRequests)
				fmt.Printf("\rProgress: %d/%d (%.1f%%)", completed, *requests, float64(completed)/float64(*requests)*100)
			}
		}
	}()

	wg.Wait()
	close(done)

	duration := time.Since(startTime)

	// Print results
	fmt.Printf("\n\nResults\n")
	fmt.Printf("=======\n")
	fmt.Printf("Total Time:       %v\n", duration)
	fmt.Printf("Total Requests:   %d\n", stats.totalRequests)
	fmt.Printf("Successful:       %d\n", stats.successfulReqs)
	fmt.Printf("Failed:           %d\n", stats.failedReqs)
	fmt.Printf("Success Rate:     %.2f%%\n", float64(stats.successfulReqs)/float64(stats.totalRequests)*100)
	fmt.Printf("Requests/sec:     %.2f\n", float64(stats.totalRequests)/duration.Seconds())

	if stats.successfulReqs > 0 {
		avgLatency := time.Duration(stats.totalLatency/stats.successfulReqs) * time.Microsecond
		fmt.Printf("Avg Latency:      %v\n", avgLatency)
		fmt.Printf("Min Latency:      %v\n", time.Duration(stats.minLatency)*time.Microsecond)
		fmt.Printf("Max Latency:      %v\n", time.Duration(stats.maxLatency)*time.Microsecond)
	}
}

func worker(work <-chan int, stats *Stats, wg *sync.WaitGroup) {
	defer wg.Done()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for range work {
		key := fmt.Sprintf("key-%d", rand.Intn(*keySpace))
		isWrite := rand.Float64() < *ratio

		start := time.Now()
		var err error

		if isWrite {
			err = doPut(client, key)
		} else {
			err = doGet(client, key)
		}

		latency := time.Since(start).Microseconds()

		atomic.AddInt64(&stats.totalRequests, 1)
		if err != nil {
			atomic.AddInt64(&stats.failedReqs, 1)
		} else {
			atomic.AddInt64(&stats.successfulReqs, 1)
			atomic.AddInt64(&stats.totalLatency, latency)

			// Update min/max (not perfectly accurate due to race, but good enough)
			for {
				old := atomic.LoadInt64(&stats.minLatency)
				if latency >= old || atomic.CompareAndSwapInt64(&stats.minLatency, old, latency) {
					break
				}
			}
			for {
				old := atomic.LoadInt64(&stats.maxLatency)
				if latency <= old || atomic.CompareAndSwapInt64(&stats.maxLatency, old, latency) {
					break
				}
			}
		}
	}
}

func doPut(client *http.Client, key string) error {
	url := fmt.Sprintf("%s/kv/%s", *target, key)
	body := map[string]string{
		"value": fmt.Sprintf("value-%d-%d", time.Now().UnixNano(), rand.Int()),
	}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequest("PUT", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func doGet(client *http.Client, key string) error {
	url := fmt.Sprintf("%s/kv/%s", *target, key)

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	// 404 is acceptable for gets (key might not exist)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
