package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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
func filamentsHandler(raftNode *raft.Raft, state *State) http.HandlerFunc {
	validTypes := map[string]bool{"PLA": true, "PETG": true, "ABS": true, "TPU": true}

	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var filament Filament
			if err := json.NewDecoder(r.Body).Decode(&filament); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			if !validTypes[filament.Type] {
				http.Error(w, "Invalid filament type", http.StatusBadRequest)
				return
			}

			cmd := map[string]interface{}{
				"op":       "add_filament",
				"filament": filament,
			}
			applyCommand(raftNode, state, w, cmd)

		case http.MethodGet:
			state.mu.Lock()
			defer state.mu.Unlock()
			json.NewEncoder(w).Encode(state.Filaments)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func printJobsHandler(raftNode *raft.Raft, state *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var job PrintJob
			if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}

			// Enforce Queued status before validation
			job.Status = "Queued"

			// Validate in a separate lock scope
			state.mu.Lock()
			_, printerExists := state.Printers[job.PrinterID]
			filament, filamentExists := state.Filaments[job.FilamentID]
			state.mu.Unlock()

			if !printerExists {
				http.Error(w, "Printer not found", http.StatusBadRequest)
				return
			}
			if !filamentExists {
				http.Error(w, "Filament not found", http.StatusBadRequest)
				return
			}

			// Calculate remaining weight without holding the lock
			state.mu.Lock()
			totalReserved := 0
			for _, j := range state.Jobs {
				if j.FilamentID == job.FilamentID && (j.Status == "Queued" || j.Status == "Running") {
					totalReserved += j.Weight
				}
			}
			state.mu.Unlock()

			if filament.Remaining-totalReserved < job.Weight {
				http.Error(w, "Not enough filament", http.StatusBadRequest)
				return
			}

			// Apply command
			cmd := map[string]interface{}{
				"op":  "add_job",
				"job": job,
			}
			applyCommand(raftNode, state, w, cmd)

		case http.MethodGet:
			state.mu.Lock()
			defer state.mu.Unlock()

			jobs := make([]PrintJob, 0, len(state.Jobs))
			for _, j := range state.Jobs {
				jobs = append(jobs, j)
			}
			json.NewEncoder(w).Encode(jobs)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
