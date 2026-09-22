package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type Conflict struct {
	Path            string
	Baseline        any
	Database        any
	File            any
	BaselinePresent bool
	DatabasePresent bool
	FilePresent     bool
}

type ConflictChoice string

const (
	ChooseDatabase ConflictChoice = "DATABASE"
	ChooseFile     ConflictChoice = "FILE"
)

// ThreeWayMerge recursively compares baseline, database and file snapshots.
// Objects are merged per member. Callers represent keyed Manifest collections
// as objects before invoking it, so independent member fields also merge.
func ThreeWayMerge(baseline any, database any, file any) (any, []Conflict) {
	value, _, conflicts := mergeAt("", baseline, true, database, true, file, true)
	return value, conflicts
}

func mergeAt(path string, baseline any, baselinePresent bool, database any, databasePresent bool, file any, filePresent bool) (any, bool, []Conflict) {
	if databasePresent == filePresent && (!databasePresent || reflect.DeepEqual(database, file)) {
		return database, databasePresent, nil
	}
	databaseChanged := databasePresent != baselinePresent || (databasePresent && !reflect.DeepEqual(database, baseline))
	fileChanged := filePresent != baselinePresent || (filePresent && !reflect.DeepEqual(file, baseline))
	if !databaseChanged {
		return file, filePresent, nil
	}
	if !fileChanged {
		return database, databasePresent, nil
	}

	baselineMap, baselineIsMap := asStringMap(baseline)
	databaseMap, databaseIsMap := asStringMap(database)
	fileMap, fileIsMap := asStringMap(file)
	if !baselinePresent {
		baselineMap = map[string]any{}
		baselineIsMap = true
	}
	if baselineIsMap && databasePresent && databaseIsMap && filePresent && fileIsMap {
		keys := unionKeys(baselineMap, databaseMap, fileMap)
		merged := make(map[string]any)
		var conflicts []Conflict
		for _, key := range keys {
			baseValue, baseFound := baselineMap[key]
			databaseValue, databaseFound := databaseMap[key]
			fileValue, fileFound := fileMap[key]
			childPath := path + "/" + escapePointer(key)
			value, present, childConflicts := mergeAt(childPath, baseValue, baseFound, databaseValue, databaseFound, fileValue, fileFound)
			conflicts = append(conflicts, childConflicts...)
			if len(childConflicts) == 0 && present {
				merged[key] = value
			}
		}
		return merged, true, conflicts
	}
	return database, databasePresent, []Conflict{{
		Path: pathOrRoot(path), Baseline: baseline, Database: database, File: file,
		BaselinePresent: baselinePresent, DatabasePresent: databasePresent, FilePresent: filePresent,
	}}
}

// ResolveThreeWay applies an explicit DATABASE or FILE decision to every
// conflict. It refuses partial and unknown decision sets.
func ResolveThreeWay(baseline any, database any, file any, choices map[string]ConflictChoice) (any, []Conflict, error) {
	merged, conflicts := ThreeWayMerge(baseline, database, file)
	if len(conflicts) == 0 {
		if len(choices) != 0 {
			return nil, nil, errors.New("conflict choices were supplied but no conflicts exist")
		}
		return merged, nil, nil
	}
	known := make(map[string]struct{}, len(conflicts))
	for _, conflict := range conflicts {
		known[conflict.Path] = struct{}{}
		choice, ok := choices[conflict.Path]
		if !ok {
			return nil, conflicts, fmt.Errorf("missing conflict choice for %s", conflict.Path)
		}
		var value any
		var present bool
		switch choice {
		case ChooseDatabase:
			value, present = conflict.Database, conflict.DatabasePresent
		case ChooseFile:
			value, present = conflict.File, conflict.FilePresent
		default:
			return nil, conflicts, fmt.Errorf("invalid conflict choice %q for %s", choice, conflict.Path)
		}
		var err error
		merged, err = setPointerValue(merged, conflict.Path, value, present)
		if err != nil {
			return nil, conflicts, err
		}
	}
	for path := range choices {
		if _, ok := known[path]; !ok {
			return nil, conflicts, fmt.Errorf("choice refers to unknown conflict %s", path)
		}
	}
	return merged, conflicts, nil
}

func setPointerValue(root any, pointer string, value any, present bool) (any, error) {
	if pointer == "/" {
		if !present {
			return nil, nil
		}
		return value, nil
	}
	parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	current, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cannot resolve %s beneath a non-object", pointer)
	}
	for index, encoded := range parts {
		part := strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
		if index == len(parts)-1 {
			if present {
				current[part] = value
			} else {
				delete(current, part)
			}
			return root, nil
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	return root, nil
}

func NormalizeJSON(data []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("normalizing JSON: %w", err)
	}
	return value, nil
}

func asStringMap(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}

func unionKeys(maps ...map[string]any) []string {
	set := make(map[string]struct{})
	for _, values := range maps {
		for key := range values {
			set[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escapePointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}

func pathOrRoot(value string) string {
	if value == "" {
		return "/"
	}
	return value
}
