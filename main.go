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
