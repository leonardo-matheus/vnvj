package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type runtimeKind string

const (
	nodeRuntime runtimeKind = "node"
	javaRuntime runtimeKind = "java"
)

type installation struct {
	Name    string
	Path    string
	Version []int
}

func discover(root string, kind runtimeKind) ([]installation, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler %s: %w", root, err)
	}

	var installations []installation
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		var version []int
		switch kind {
		case nodeRuntime:
			if _, err := os.Stat(filepath.Join(path, "node.exe")); err != nil {
				continue
			}
			version = parseVersion(strings.TrimPrefix(strings.ToLower(entry.Name()), "v"))
		case javaRuntime:
			if _, err := os.Stat(filepath.Join(path, "bin", "java.exe")); err != nil {
				continue
			}
			version, err = readJavaVersion(filepath.Join(path, "release"))
			if err != nil {
				continue
			}
		}
		if len(version) > 0 {
			installations = append(installations, installation{Name: entry.Name(), Path: path, Version: version})
		}
	}

	sort.Slice(installations, func(i, j int) bool {
		return compareVersions(installations[i].Version, installations[j].Version) > 0
	})
	return installations, nil
}

func readJavaVersion(path string) ([]int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "JAVA_VERSION=") {
			continue
		}
		value := strings.Trim(strings.TrimPrefix(line, "JAVA_VERSION="), "\"")
		version := parseVersion(value)
		if len(version) >= 2 && version[0] == 1 {
			version = version[1:]
		}
		if len(version) == 0 {
			return nil, fmt.Errorf("JAVA_VERSION inválida em %s", path)
		}
		return version, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("JAVA_VERSION ausente em %s", path)
}

func parseVersion(value string) []int {
	var parts []int
	for _, field := range strings.FieldsFunc(value, func(r rune) bool { return r < '0' || r > '9' }) {
		n, err := strconv.Atoi(field)
		if err != nil {
			return nil
		}
		parts = append(parts, n)
	}
	return parts
}

func compareVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func parseMajor(value string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("informe apenas o número principal da versão")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("versão inválida %q; informe apenas o número principal", value)
		}
	}
	major, err := strconv.Atoi(value)
	if err != nil || major < 1 {
		return 0, fmt.Errorf("versão inválida %q; informe um número positivo", value)
	}
	return major, nil
}

func resolveInstalled(installations []installation, major int) (installation, bool) {
	for _, candidate := range installations {
		if len(candidate.Version) > 0 && candidate.Version[0] == major {
			return candidate, true
		}
	}
	return installation{}, false
}

func versionString(version []int) string {
	parts := make([]string, len(version))
	for i, part := range version {
		parts[i] = strconv.Itoa(part)
	}
	return strings.Join(parts, ".")
}
