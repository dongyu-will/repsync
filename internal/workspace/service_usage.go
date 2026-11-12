package workspace

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// LocalServiceUsage is an on-demand host process snapshot, never a container metric.
type LocalServiceUsage struct {
	CPUPercent   float64 `json:"cpuPercent"`
	MemoryBytes  int64   `json:"memoryBytes"`
	ProcessCount int     `json:"processCount"`
	Scope        string  `json:"scope"`
	SampledAt    string  `json:"sampledAt"`
}

type processUsage struct {
	pid, group int
	cpu        float64
	rss        int64
}

func readProcessUsage() ([]processUsage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,pgid=,pcpu=,rss=")
	cmd.Env = append(commandEnvironment(false), "LC_ALL=C")
	var out, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("read host process usage: %w", err)
	}
	if out.overflow {
		return nil, errors.New("host process usage exceeded the snapshot size limit")
	}
	processes := []processUsage{}
	for _, line := range strings.Split(out.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 4 {
			return nil, errors.New("host process usage returned an incomplete row")
		}
		pid, pidErr := strconv.Atoi(fields[0])
		group, groupErr := strconv.Atoi(fields[1])
		cpu, cpuErr := strconv.ParseFloat(fields[2], 64)
		rss, rssErr := strconv.ParseInt(fields[3], 10, 64)
		invalidCPU := cpu < 0 || math.IsNaN(cpu) || math.IsInf(cpu, 0)
		if pidErr != nil || groupErr != nil || cpuErr != nil || rssErr != nil || invalidCPU || rss < 0 {
			return nil, errors.New("host process usage returned invalid measurements")
		}
		processes = append(processes, processUsage{pid: pid, group: group, cpu: cpu, rss: rss * 1024})
	}
	return processes, nil
}

func populateServiceUsage(states []LocalServiceState) {
	needed := false
	for i := range states {
		states[i].Usage, states[i].UsageError = nil, ""
		needed = needed || localServiceRunning(states[i].Status) ||
			(states[i].Definition.ObserveOnly && states[i].PortState == "listening")
	}
	if !needed {
		return
	}
	processes, sampleErr := readProcessUsage()
	sampledAt := time.Now().UTC().Format(time.RFC3339)
	for i := range states {
		state := &states[i]
		observed := state.Definition.ObserveOnly && state.PortState == "listening"
		if !localServiceRunning(state.Status) && !observed {
			continue
		}
		if sampleErr != nil {
			state.UsageError = sampleErr.Error()
			continue
		}
		usage := &LocalServiceUsage{Scope: "process-group", SampledAt: sampledAt}
		listeners := map[int]bool{}
		if observed {
			usage.Scope = "listener"
			owners, err := localPortOwners(state.Definition.Port)
			if err != nil {
				state.UsageError = err.Error()
				continue
			}
			for _, owner := range owners {
				listeners[owner.PID] = true
			}
		}
		// Only include a managed group while its leader is present in this snapshot.
		owned := false
		for _, process := range processes {
			if state.PID > 0 && process.pid == state.PID && process.group == state.PID {
				owned = true
				break
			}
		}
		for _, process := range processes {
			if (observed && listeners[process.pid]) || (!observed && owned && process.group == state.PID) {
				usage.CPUPercent += process.cpu
				usage.MemoryBytes += process.rss
				usage.ProcessCount++
			}
		}
		if usage.ProcessCount == 0 {
			state.UsageError = "No matching host process was available in the snapshot"
			continue
		}
		state.Usage = usage
	}
}
