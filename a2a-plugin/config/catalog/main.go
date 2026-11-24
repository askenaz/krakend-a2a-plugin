// Copyright 2026 Los autores del plugin A2A para KrakenD.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type object = map[string]json.RawMessage

var agentID = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

var sections = []string{"card", "versions", "security", "capabilities", "disabled_operations", "ownership", "gateway"}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: a2a-catalog <settings-directory>")
		os.Exit(2)
	}
	settings, err := assemble(os.Args[1])
	if err == nil {
		err = json.NewEncoder(os.Stdout).Encode(settings)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "a2a-catalog:", err)
		os.Exit(1)
	}
}

func assemble(dir string) (object, error) {
	global := filepath.Join(dir, "a2a.json")
	settings, err := readObject(global)
	if err != nil {
		return nil, err
	}

	agents := make([]object, 0)
	sources := make(map[string]string)
	add := func(agent object, expectedID, source string) error {
		var id string
		if err := json.Unmarshal(agent["id"], &id); err != nil || !agentID.MatchString(id) {
			return fmt.Errorf("%s: invalid agent id", source)
		}
		if expectedID != "" && id != expectedID {
			return fmt.Errorf("%s: agent id %q must match %q", source, id, expectedID)
		}
		if other, exists := sources[id]; exists {
			return fmt.Errorf("duplicate agent id %q in %s and %s", id, other, source)
		}
		sources[id] = source
		agents = append(agents, agent)
		return nil
	}

	if raw, exists := settings["agents"]; exists {
		var inline []object
		if err := json.Unmarshal(raw, &inline); err != nil || inline == nil {
			return nil, fmt.Errorf("%s: agents must be an array of objects", global)
		}
		for i, agent := range inline {
			if err := add(agent, "", fmt.Sprintf("%s: agents[%d]", global, i)); err != nil {
				return nil, err
			}
		}
	}

	catalog := filepath.Join(dir, "agents")
	entries, err := os.ReadDir(catalog)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(catalog, entry.Name())
		id := entry.Name()
		var agent object
		switch {
		case entry.IsDir():
			path = filepath.Join(path, "agent.json")
			agent, err = readAgentFolder(path)
		case filepath.Ext(id) == ".json":
			id = strings.TrimSuffix(id, ".json")
			agent, err = readObject(path)
			if err == nil {
				err = checkAgentFields(path, agent)
			}
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		if !agentID.MatchString(id) {
			return nil, fmt.Errorf("%s: invalid agent file or directory name %q", path, id)
		}
		if err := add(agent, id, path); err != nil {
			return nil, err
		}
	}
	if len(agents) == 0 {
		return nil, fmt.Errorf("%s: no agents defined", dir)
	}
	raw, err := json.Marshal(agents)
	if err != nil {
		return nil, err
	}
	settings["agents"] = raw
	return settings, nil
}

func readAgentFolder(manifest string) (object, error) {
	agent, err := readObject(manifest)
	if err != nil {
		return nil, err
	}
	if err := checkAgentFields(manifest, agent); err != nil {
		return nil, err
	}
	dir := filepath.Dir(manifest)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == "agent.json" || filepath.Ext(name) != ".json" {
			continue
		}
		path := filepath.Join(dir, name)
		field := strings.TrimSuffix(name, ".json")
		if !slices.Contains(sections, field) {
			return nil, fmt.Errorf("%s: unknown agent section %q", path, field)
		}
		if _, exists := agent[field]; exists {
			return nil, fmt.Errorf("agent section %q is defined in both %s and %s", field, manifest, path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if !json.Valid(raw) {
			return nil, fmt.Errorf("%s: invalid JSON", path)
		}
		if err := checkSection(path, field, raw); err != nil {
			return nil, err
		}
		agent[field] = raw
	}
	return agent, nil
}

func checkAgentFields(path string, agent object) error {
	for field, value := range agent {
		if field == "id" {
			continue
		}
		if !slices.Contains(sections, field) {
			return fmt.Errorf("%s: unknown agent field %q", path, field)
		}
		if err := checkSection(path, field, value); err != nil {
			return err
		}
	}
	return nil
}

func checkSection(path, field string, raw json.RawMessage) error {
	expected := byte('{')
	shape := "an object"
	if field == "versions" || field == "disabled_operations" {
		expected = '['
		shape = "an array"
	}
	// Values from both sources have already passed JSON decoding.
	if bytes.TrimSpace(raw)[0] != expected {
		return fmt.Errorf("%s: %s must be %s", path, field, shape)
	}
	return nil
}

func readObject(path string) (object, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value object
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if value == nil {
		return nil, fmt.Errorf("%s: expected a JSON object", path)
	}
	return value, nil
}
