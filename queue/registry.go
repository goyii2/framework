package queue

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// Registry is the application's job table: name -> prototype. Workers rebuild
// jobs through it — Build decodes the pushed payload into a fresh instance of
// the prototype's concrete type — so push sides only ever store a name and a
// JSON payload. The zero value is an empty, ready registry. Safe for
// concurrent use.
type Registry struct {
	mu   sync.RWMutex
	jobs map[string]Job
}

// NewRegistry builds a registry preloaded with jobs, rejecting duplicate names
// and invalid registrations (nil job, empty name, non-pointer job).
func NewRegistry(jobs ...Job) (*Registry, error) {
	r := new(Registry)
	for _, job := range jobs {
		if err := r.Register(job); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds job under its name. The job must be a pointer: the registry
// instantiates its concrete type when rebuilding payloads, and only pointers
// give it one to instantiate.
func (r *Registry) Register(job Job) error {
	v := reflect.ValueOf(job)
	if job == nil || (v.Kind() == reflect.Pointer && v.IsNil()) {
		return fmt.Errorf("queue: register nil job")
	}
	name := job.Name()
	if name == "" {
		return fmt.Errorf("queue: register job with empty name (%T)", job)
	}
	if v.Kind() != reflect.Pointer {
		return fmt.Errorf("queue: job %q must be registered as a pointer (got %T)", name, job)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.jobs == nil {
		r.jobs = make(map[string]Job)
	}
	if _, dup := r.jobs[name]; dup {
		return fmt.Errorf("queue: job %q is already registered (%T)", name, job)
	}
	r.jobs[name] = job
	return nil
}

// Get returns the prototype registered under name.
func (r *Registry) Get(name string) (Job, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.jobs[name]
	return job, ok
}

// Names lists the registered job names sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.jobs))
	for name := range r.jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Build rebuilds the job registered under name from payload: a fresh instance
// of the prototype's type with the payload decoded into it. The prototype
// itself is never returned, so a rebuild can never leak state between
// executions. Register's checks make the reflection here safe: name resolves
// to a non-nil pointer whose type implements Job.
func (r *Registry) Build(name string, payload []byte) (Job, error) {
	proto, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("queue: job %q is not registered", name)
	}
	job, ok := reflect.New(reflect.TypeOf(proto).Elem()).Interface().(Job)
	if !ok {
		return nil, fmt.Errorf("queue: job %q prototype does not rebuild to a Job", name)
	}
	if err := json.Unmarshal(payload, job); err != nil {
		return nil, fmt.Errorf("queue: decode payload of %q: %w", name, err)
	}
	return job, nil
}
