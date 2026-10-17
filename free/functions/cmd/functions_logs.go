package main

// Purpose: `nself functions logs` split out of functions.go (CLI-R12
// Batch B mechanical file-size split). Tails or streams the functions
// container's docker logs, filtered to lines mentioning the given
// function name.
// Inputs: cobra command flags (--follow, --since, --tail) and the
// positional function name.
// Outputs: matching log lines written to stdout as they are read. With
// --json (not with --follow), one v1 envelope (data: name, count, lines) or
// an error envelope (E764 docker logs failed); see envelope.go.
// Constraints: human output unchanged. Lines from the container's stdout and
// stderr streams are merged in arrival order. functionNamePattern and
// functionsCmd (parent) remain in functions.go.

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

var functionsLogsFlags struct {
	follow bool
	since  string
	tail   int
}

var functionsLogsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Stream logs for a function",
	Long: `Tail or stream logs for a deployed function, filtered by function name.

Examples:
  nself functions logs hello-world
  nself functions logs hello-world --follow
  nself functions logs hello-world --tail 50
  nself functions logs hello-world --since 1h`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsLogs,
}

func init() {
	functionsLogsCmd.Flags().BoolVar(&functionsLogsFlags.follow, "follow", false, "Stream logs continuously")
	functionsLogsCmd.Flags().StringVar(&functionsLogsFlags.since, "since", "", "Show logs since duration (e.g. 1h, 30m)")
	functionsLogsCmd.Flags().IntVar(&functionsLogsFlags.tail, "tail", 100, "Number of recent log lines")
	addJSONFlag(functionsLogsCmd)
}

func runFunctionsLogs(cmd *cobra.Command, args []string) error {
	r := begin(cmd, false)
	defer r.end()
	name := args[0]
	if !functionNamePattern.MatchString(name) {
		return r.usageSafe(fmt.Sprintf("invalid function name %q", name), "invalid function name")
	}
	if r.json && functionsLogsFlags.follow {
		return r.usage("--json cannot be combined with --follow: a stream is not one envelope")
	}

	cfg, _, cfgErr := loadHealthConfig()
	containerID := "functions"
	if cfgErr == nil {
		containerID = fmt.Sprintf("%s_functions", cfg.ProjectName)
	}

	dockerArgs := []string{"logs"}
	if functionsLogsFlags.follow {
		dockerArgs = append(dockerArgs, "--follow")
	}
	if functionsLogsFlags.since != "" {
		dockerArgs = append(dockerArgs, "--since", functionsLogsFlags.since)
	}
	dockerArgs = append(dockerArgs, fmt.Sprintf("--tail=%d", functionsLogsFlags.tail))
	dockerArgs = append(dockerArgs, containerID)

	logsCmd := exec.CommandContext(cmd.Context(), "docker", dockerArgs...)
	stdout, err := logsCmd.StdoutPipe()
	if err != nil {
		return r.fail(logsErr(fmt.Errorf("pipe: %w", err)))
	}
	stderr, err := logsCmd.StderrPipe()
	if err != nil {
		return r.fail(logsErr(fmt.Errorf("stderr pipe: %w", err)))
	}

	if err := logsCmd.Start(); err != nil {
		return r.fail(logsErr(fmt.Errorf("starting docker logs: %w", err)))
	}

	// Filter lines containing the function name and print (JSON mode: collect).
	var mu sync.Mutex
	lines := []string{}
	filterAndPrint := func(rd io.Reader) {
		scanner := bufio.NewScanner(rd)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, name) || strings.Contains(line, "/v1/"+name) {
				if r.json {
					mu.Lock()
					lines = append(lines, line)
					mu.Unlock()
					continue
				}
				fmt.Println(line)
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); filterAndPrint(stderr) }()
	filterAndPrint(stdout)
	wg.Wait()

	if err := logsCmd.Wait(); err != nil {
		return r.fail(logsErr(err))
	}
	return r.done(map[string]any{"name": name, "count": len(lines), "lines": lines})
}

// logsErr is E764 for a failed docker logs run.
func logsErr(err error) error {
	return newErr(err, CodeLogsFailed, output.ClassInfra, "docker logs for the functions container failed",
		"check that docker is running and the functions service is enabled")
}
