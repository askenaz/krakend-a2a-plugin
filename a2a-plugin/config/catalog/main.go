// Copyright 2026 Los autores del plugin A2A para KrakenD.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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

// settingsDir reads only inside the settings directory. Symbolic links that
// stay inside it work, as in Kubernetes ConfigMap volumes. A link that leads
// outside it is an error, so the catalog never reads other files.
type settingsDir struct {
	dir  string
	fsys fs.FS
}

// path is the name shown in messages for a slash-separated name inside dir.
func (s settingsDir) path(name string) string {
	return filepath.Join(s.dir, filepath.FromSlash(name))
}

func (s settingsDir) readFile(name string) ([]byte, error) {
	data, err := fs.ReadFile(s.fsys, name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.path(name), err)
	}
	return data, nil
}

func assemble(dir string) (object, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	s := settingsDir{dir: dir, fsys: root.FS()}

	settings, err := s.readObject("a2a.json")
	if err != nil {
		return nil, err
	}
	global := s.path("a2a.json")

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

	entries, err := fs.ReadDir(s.fsys, "agents")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", s.path("agents"), err)
	}
	for _, entry := range entries {
		name := path.Join("agents", entry.Name())
		id := entry.Name()
		var agent object
		switch {
		case entry.IsDir():
			name = path.Join(name, "agent.json")
			agent, err = s.readAgentFolder(name)
		case filepath.Ext(id) == ".json":
			id = strings.TrimSuffix(id, ".json")
			agent, err = s.readObject(name)
			if err == nil {
				err = checkAgentFields(s.path(name), agent)
			}
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		if !agentID.MatchString(id) {
			return nil, fmt.Errorf("%s: invalid agent file or directory name %q", s.path(name), id)
		}
		if err := add(agent, id, s.path(name)); err != nil {
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

func (s settingsDir) readAgentFolder(manifest string) (object, error) {
	agent, err := s.readObject(manifest)
	if err != nil {
		return nil, err
	}
	if err := checkAgentFields(s.path(manifest), agent); err != nil {
		return nil, err
	}
	folder := path.Dir(manifest)
	entries, err := fs.ReadDir(s.fsys, folder)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.path(folder), err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == "agent.json" || filepath.Ext(name) != ".json" {
			continue
		}
		file := path.Join(folder, name)
		field := strings.TrimSuffix(name, ".json")
		if !slices.Contains(sections, field) {
			return nil, fmt.Errorf("%s: unknown agent section %q", s.path(file), field)
		}
		if _, exists := agent[field]; exists {
			return nil, fmt.Errorf("agent section %q is defined in both %s and %s", field, s.path(manifest), s.path(file))
		}
		raw, err := s.readFile(file)
		if err != nil {
			return nil, err
		}
		if !json.Valid(raw) {
			return nil, fmt.Errorf("%s: invalid JSON", s.path(file))
		}
		if err := checkSection(s.path(file), field, raw); err != nil {
			return nil, err
		}
		agent[field] = raw
	}
	return agent, nil
}

func checkAgentFields(source string, agent object) error {
	for field, value := range agent {
		if field == "id" {
			continue
		}
		if !slices.Contains(sections, field) {
			return fmt.Errorf("%s: unknown agent field %q", source, field)
		}
		if err := checkSection(source, field, value); err != nil {
			return err
		}
	}
	return nil
}

func checkSection(source, field string, raw json.RawMessage) error {
	expected := byte('{')
	shape := "an object"
	if field == "versions" || field == "disabled_operations" {
		expected = '['
		shape = "an array"
	}
	// Values from both sources have already passed JSON decoding.
	if bytes.TrimSpace(raw)[0] != expected {
		return fmt.Errorf("%s: %s must be %s", source, field, shape)
	}
	return nil
}

func (s settingsDir) readObject(name string) (object, error) {
	data, err := s.readFile(name)
	if err != nil {
		return nil, err
	}
	var value object
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path(name), err)
	}
	if value == nil {
		return nil, fmt.Errorf("%s: expected a JSON object", s.path(name))
	}
	return value, nil
}
