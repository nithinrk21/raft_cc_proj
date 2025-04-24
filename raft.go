package main

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
	"strconv"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

// Data Structures
type Printer struct {
	ID      string `json:"id"`
	Company string `json:"company"`
	Model   string `json:"model"`
}

type Filament struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // PLA, PETG, ABS, TPU
	Color       string `json:"color"`
	TotalWeight int    `json:"total_weight_in_grams"`
	Remaining   int    `json:"remaining_weight_in_grams"`
}

type PrintJob struct {
	ID         string `json:"id"`
	PrinterID  string `json:"printer_id"`
	FilamentID string `json:"filament_id"`
	Filepath   string `json:"filepath"`
	Weight     int    `json:"print_weight_in_grams"`
	Status     string `json:"status"` // Queued, Running, Done, Canceled
}

type State struct {
	Printers  map[string]Printer  `json:"printers"`
	Filaments map[string]Filament `json:"filaments"`
	Jobs      map[string]PrintJob `json:"jobs"`
	mu        sync.Mutex
}

// Raft FSM Implementation
func (s *State) Apply(log *raft.Log) interface{} {
	var cmd struct {
		Op       string      `json:"op"`
		Printer  Printer     `json:"printer,omitempty"`
		Filament Filament    `json:"filament,omitempty"`
		Job      PrintJob    `json:"job,omitempty"`
		JobID    string      `json:"job_id,omitempty"`
		Status   string      `json:"status,omitempty"`
	}
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch cmd.Op {
	case "add_printer":
		s.Printers[cmd.Printer.ID] = cmd.Printer
	case "add_filament":
		s.Filaments[cmd.Filament.ID] = cmd.Filament
	case "add_job":
		cmd.Job.Status = "Queued"
		s.Jobs[cmd.Job.ID] = cmd.Job
	case "update_job":
		if job, exists := s.Jobs[cmd.JobID]; exists {
			if cmd.Job.Status == "Done" {
				if filament, ok := s.Filaments[job.FilamentID]; ok {
					filament.Remaining -= job.Weight
					s.Filaments[job.FilamentID] = filament
				}
			}
			job.Status = cmd.Job.Status
			s.Jobs[cmd.JobID] = job
		}
	}
	return nil
}

func (s *State) Snapshot() (raft.FSMSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := &StateSnapshot{
		Printers:  make(map[string]Printer),
		Filaments: make(map[string]Filament),
		Jobs:      make(map[string]PrintJob),
	}
	for k, v := range s.Printers {
		snapshot.Printers[k] = v
	}
	for k, v := range s.Filaments {
		snapshot.Filaments[k] = v
	}
	for k, v := range s.Jobs {
		snapshot.Jobs[k] = v
	}
	return snapshot, nil
}

func (s *State) Restore(rc io.ReadCloser) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer rc.Close()

	s.Printers = make(map[string]Printer)
	s.Filaments = make(map[string]Filament)
	s.Jobs = make(map[string]PrintJob)
	return json.NewDecoder(rc).Decode(s)
}

type StateSnapshot struct {
	Printers  map[string]Printer
	Filaments map[string]Filament
	Jobs      map[string]PrintJob
}

func (s *StateSnapshot) Persist(sink raft.SnapshotSink) error {
	return json.NewEncoder(sink).Encode(s)
}

func (s *StateSnapshot) Release() {}

// Raft Setup
func SetupRaft(nodeID, dataDir string, raftPort int, bootstrap bool) (*raft.Raft, *State, error) {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)
	config.SnapshotInterval = 30 * time.Second
	config.SnapshotThreshold = 1000

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, nil, err
	}

	logStore, err := raftboltdb.NewBoltStore(dataDir + "/raft-log.bolt")
	if err != nil {
		return nil, nil, err
	}

	stableStore, err := raftboltdb.NewBoltStore(dataDir + "/raft-stable.bolt")
	if err != nil {
		return nil, nil, err
	}

	snapshots, err := raft.NewFileSnapshotStore(dataDir, 1, os.Stdout)
	if err != nil {
		return nil, nil, err
	}

	addr := "127.0.0.1:" + strconv.Itoa(raftPort)
	transport, err := raft.NewTCPTransport(addr, nil, 3, 10*time.Second, os.Stdout)
	if err != nil {
		return nil, nil, err
	}

	state := &State{
		Printers:  make(map[string]Printer),
		Filaments: make(map[string]Filament),
		Jobs:      make(map[string]PrintJob),
	}

	raftNode, err := raft.NewRaft(config, state, logStore, stableStore, snapshots, transport)
	if err != nil {
		return nil, nil, err
	}

	if bootstrap {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      config.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		future := raftNode.BootstrapCluster(configuration)
		if err := future.Error(); err != nil {
			return nil, nil, err
		}
	}

	return raftNode, state, nil
}
