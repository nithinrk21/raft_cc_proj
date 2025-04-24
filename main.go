package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"

	"github.com/hashicorp/raft"
)

func main() {
	// Configuration
	id := flag.String("id", "node1", "Raft node ID")
	raftPort := flag.Int("raft-port", 5000, "Raft TCP port")
	httpPort := flag.Int("http-port", 8080, "HTTP API port")
	bootstrap := flag.Bool("bootstrap", false, "Bootstrap cluster")
	flag.Parse()

	// Initialize Raft
	dataDir := "./raft_data_" + *id
	raftNode, state, err := SetupRaft(*id, dataDir, *raftPort, *bootstrap)
	if err != nil {
		log.Fatalf("Failed to start Raft: %v", err)
	}

	// API Endpoints
	http.HandleFunc("/api/v1/printers", printersHandler(raftNode, state))
	http.HandleFunc("/api/v1/filaments", filamentsHandler(raftNode, state))
	http.HandleFunc("/api/v1/print_jobs", printJobsHandler(raftNode, state))
	http.HandleFunc("/api/v1/print_jobs/", printJobStatusHandler(raftNode, state))

	// Cluster Management
	http.HandleFunc("/cluster", clusterHandler(raftNode))
	http.HandleFunc("/cluster/add", clusterAddHandler(raftNode))
	http.HandleFunc("/cluster/remove", clusterRemoveHandler(raftNode))

	// Start Server
	log.Printf("Starting server on port %d (Raft port %d)", *httpPort, *raftPort)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*httpPort), nil))
}

// Handlers
func printersHandler(raftNode *raft.Raft, state *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var printer Printer
			if err := json.NewDecoder(r.Body).Decode(&printer); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			if printer.ID == "" || printer.Company == "" || printer.Model == "" {
				http.Error(w, "Missing required fields", http.StatusBadRequest)
				return
			}

			cmd := map[string]interface{}{
				"op":      "add_printer",
				"printer": printer,
			}
			applyCommand(raftNode, state, w, cmd)

		case http.MethodGet:
			state.mu.Lock()
			defer state.mu.Unlock()

			printers := make([]Printer, 0, len(state.Printers))
			for _, p := range state.Printers {
				printers = append(printers, p)
			}
			json.NewEncoder(w).Encode(printers)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
