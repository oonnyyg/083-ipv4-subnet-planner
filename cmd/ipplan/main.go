package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"example.com/ipv4-subnet-planner/internal/planner"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("ipplan", flag.ContinueOnError)
	in := fs.String("in", "", "path to the JSON task file")
	out := fs.String("out", "", "path to the JSON plan file to write")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if *in == "" && len(rest) > 0 {
		*in = rest[0]
	}
	if *out == "" && len(rest) > 1 {
		*out = rest[1]
	}
	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: ipplan -in task.json -out plan.json")
		return 2
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		return writeFailure(*out, planner.Failure{
			Object: "task", Reason: "cannot read input file: " + err.Error(),
		})
	}
	var task planner.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return writeFailure(*out, planner.Failure{
			Object: "task", Reason: "invalid JSON: " + err.Error(),
		})
	}

	plan, failure := planner.Run(task)
	if failure != nil {
		return writeFailure(*out, *failure)
	}
	if err := writeJSON(*out, plan); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// writeFailure reports the failure into the output file (never a partial
// plan) and yields a non-zero exit code.
func writeFailure(path string, f planner.Failure) int {
	if err := writeJSON(path, map[string]any{"error": f}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "planning failed: %s %s: %s\n", f.Object, f.ID, f.Reason)
	return 1
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
