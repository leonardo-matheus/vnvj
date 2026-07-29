package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLatestInstalledMajor(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"v18.12.0", "v18.18.0", "v22.12.0"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "node.exe"), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	installations, err := discover(root, nodeRuntime)
	if err != nil {
		t.Fatal(err)
	}
	selected, ok := resolveInstalled(installations, 18)
	if !ok || selected.Name != "v18.18.0" {
		t.Fatalf("esperava v18.18.0, obteve %#v", selected)
	}
}

func TestResolveJavaEightFromRelease(t *testing.T) {
	root := t.TempDir()
	versions := map[string]string{
		"jdk1.8.0_201": "JAVA_VERSION=\"1.8.0_201\"\n",
		"jdk8u412-b08": "JAVA_VERSION=\"1.8.0_412\"\n",
		"jdk-11.0.20":  "JAVA_VERSION=\"11.0.20\"\n",
	}
	for name, release := range versions {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", "java.exe"), nil, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "release"), []byte(release), 0644); err != nil {
			t.Fatal(err)
		}
	}
	installations, err := discover(root, javaRuntime)
	if err != nil {
		t.Fatal(err)
	}
	selected, ok := resolveInstalled(installations, 8)
	if !ok || selected.Name != "jdk8u412-b08" {
		t.Fatalf("esperava jdk8u412-b08, obteve %#v", selected)
	}
}

func TestParseMajorRejectsFullVersion(t *testing.T) {
	if _, err := parseMajor("18.18.0"); err == nil {
		t.Fatal("esperava rejeição da versão completa")
	}
}
