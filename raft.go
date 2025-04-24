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
	ID      string json:"id"
	Company string json:"company"
	Model   string json:"model"
}

type Filament struct {
	ID          string json:"id"
	Type        string json:"type" // PLA, PETG, ABS, TPU
	Color       string json:"color"
	TotalWeight int    json:"total_weight_in_grams"
	Remaining   int    json:"remaining_weight_in_grams"
}

type PrintJob struct {
	ID         string json:"id"
	PrinterID  string json:"printer_id"
	FilamentID string json:"filament_id"
	Filepath   string json:"filepath"
	Weight     int    json:"print_weight_in_grams"
	Status     string json:"status" // Queued, Running, Done, Canceled
}

type State struct {
	Printers  map[string]Printer  json:"printers"
	Filaments map[string]Filament json:"filaments"
	Jobs      map[string]PrintJob json:"jobs"
	mu        sync.Mutex
}
// Raft FSM Implementation
func (s *State) Apply(log *raft.Log) interface{} {
	var cmd struct {
		Op       string      json:"op"
		Printer  Printer     json:"printer,omitempty"
		Filament Filament    json:"filament,omitempty"
		Job      PrintJob    json:"job,omitempty"
		JobID    string      json:"job_id,omitempty"
		Status   string      json:"status,omitempty"
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
