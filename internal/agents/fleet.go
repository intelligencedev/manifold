package agents

// FleetWorker defines a basic agent worker in the fleet.
type FleetWorker struct {
	Name         string  `json:"name"`
	Role         string  `json:"role"`
	Endpoint     string  `json:"endpoint"`
	Model        string  `json:"model,omitempty"`
	CtxSize      int     `json:"ctx_size"`
	Temperature  float32 `json:"temperature"`
	Instructions string  `json:"instructions"`
}

// Fleet defines a collection of workers in the fleet.
type Fleet struct {
	Workers []FleetWorker `json:"workers"`
}

// NewFleet creates a new Fleet instance.
func NewFleet() *Fleet {
	return &Fleet{
		Workers: []FleetWorker{},
	}
}

// AddWorker adds a new worker to the fleet.
func (f *Fleet) AddWorker(worker FleetWorker) {
	f.Workers = append(f.Workers, worker)
}

// GetWorker retrieves a worker by name.
func (f *Fleet) GetWorker(name string) *FleetWorker {
	for _, worker := range f.Workers {
		if worker.Name == name {
			return &worker
		}
	}
	return nil // or return an error if preferred
}

// RemoveWorker removes a worker from the fleet by name.
func (f *Fleet) RemoveWorker(name string) {
	for i, worker := range f.Workers {
		if worker.Name == name {
			f.Workers = append(f.Workers[:i], f.Workers[i+1:]...)
			return
		}
	}
	// Optionally return an error if the worker was not found
}

// ListWorkers returns a list of all workers in the fleet.
func (f *Fleet) ListWorkers() []FleetWorker {
	return f.Workers
}

// UpdateWorker updates an existing worker's details.
func (f *Fleet) UpdateWorker(name string, updatedWorker FleetWorker) bool {
	for i, worker := range f.Workers {
		if worker.Name == name {
			f.Workers[i] = updatedWorker
			return true // Update successful
		}
	}
	return false // Worker not found
}

// ClearWorkers removes all workers from the fleet.
func (f *Fleet) ClearWorkers() {
	f.Workers = []FleetWorker{}
}

// CountWorkers returns the number of workers in the fleet.
func (f *Fleet) CountWorkers() int {
	return len(f.Workers)
}

// FindWorkersByRole returns a list of workers matching a specific role.
func (f *Fleet) FindWorkersByRole(role string) []FleetWorker {
	var matchedWorkers []FleetWorker
	for _, worker := range f.Workers {
		if worker.Role == role {
			matchedWorkers = append(matchedWorkers, worker)
		}
	}
	return matchedWorkers
}

// FindWorkersByName returns a list of workers matching a specific name.
func (f *Fleet) FindWorkersByName(name string) []FleetWorker {
	var matchedWorkers []FleetWorker
	for _, worker := range f.Workers {
		if worker.Name == name {
			matchedWorkers = append(matchedWorkers, worker)
		}
	}
	return matchedWorkers
}
