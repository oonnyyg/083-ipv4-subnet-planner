package main

import (
	"encoding/json"
	"fmt"
	"os"

	"example.com/ipv4-subnet-planner/internal/planner"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: ipplan <task.json> <plan.json>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		if oe, ok := err.(*planner.ObjectError); ok {
			out, _ := json.Marshal(map[string]string{
				"error":  oe.Msg,
				"kind":   oe.Kind,
				"object": oe.ID,
			})
			fmt.Fprintln(os.Stderr, string(out))
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run(inPath, outPath string) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("read task: %w", err)
	}
	var task planner.Task
	if err := json.Unmarshal(data, &task); err != nil {
		return fmt.Errorf("parse task JSON: %w", err)
	}
	plan, err := planner.PlanTask(&task)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	return nil
}
