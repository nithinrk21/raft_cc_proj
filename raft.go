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
	mu    	  sync.Mutex
}
