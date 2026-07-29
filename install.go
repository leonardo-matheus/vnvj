package main

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	nodeIndexURL = "https://nodejs.org/dist/index.json"
	adoptiumURL  = "https://api.adoptium.net/v3/assets/latest/%d/hotspot?architecture=x64&heap_size=normal&image_type=jdk&os=windows&vendor=eclipse"
)

type artifact struct {
	Version    string
	URL        string
	Checksum   string
	Name       string
	TargetName string
}

func ensureInstalled(kind runtimeKind, root string, major int) (installation, error) {
	installations, err := discover(root, kind)
	if err != nil {
		return installation{}, err
	}
	if selected, ok := resolveInstalled(installations, major); ok {
		return selected, nil
	}

	remote, err := findArtifact(kind, major)
	if err != nil {
		return installation{}, err
	}
	label := "Node.js"
	if kind == javaRuntime {
		label = "Java Temurin"
	}
	fmt.Printf("%s %d não está instalado. Instalar %s? [S/N]: ", label, major, remote.Version)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return installation{}, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "s" && answer != "sim" && answer != "y" && answer != "yes" {
		return installation{}, errors.New("instalação cancelada")
	}
	if err := installArtifact(root, remote); err != nil {
		return installation{}, err
	}
	installations, err = discover(root, kind)
	if err != nil {
		return installation{}, err
	}
	selected, ok := resolveInstalled(installations, major)
	if !ok {
		return installation{}, errors.New("a instalação terminou, mas a versão não foi encontrada")
	}
	return selected, nil
}

func findArtifact(kind runtimeKind, major int) (artifact, error) {
	if kind == nodeRuntime {
		return findNodeArtifact(major)
	}
	return findJavaArtifact(major)
}

func findNodeArtifact(major int) (artifact, error) {
	var releases []struct {
		Version string   `json:"version"`
		Files   []string `json:"files"`
	}
	if err := getJSON(nodeIndexURL, &releases); err != nil {
		return artifact{}, fmt.Errorf("não foi possível consultar as versões do Node.js: %w", err)
	}
	var selected []int
	var version string
	for _, release := range releases {
		parsed := parseVersion(strings.TrimPrefix(release.Version, "v"))
		if len(parsed) == 0 || parsed[0] != major || !slices.Contains(release.Files, "win-x64-zip") {
			continue
		}
		if version == "" || compareVersions(parsed, selected) > 0 {
			selected, version = parsed, release.Version
		}
	}
	if version == "" {
		return artifact{}, fmt.Errorf("não existe uma versão oficial do Node.js %d para Windows x64", major)
	}
	name := "node-" + version + "-win-x64.zip"
	base := "https://nodejs.org/dist/" + version + "/"
	checksums, err := getText(base + "SHASUMS256.txt")
	if err != nil {
		return artifact{}, fmt.Errorf("não foi possível consultar o checksum do Node.js: %w", err)
	}
	checksum := checksumFor(checksums, name)
	if checksum == "" {
		return artifact{}, fmt.Errorf("checksum ausente para %s", name)
	}
	return artifact{
		Version:    strings.TrimPrefix(version, "v"),
		URL:        base + name,
		Checksum:   checksum,
		Name:       name,
		TargetName: version,
	}, nil
}

func findJavaArtifact(major int) (artifact, error) {
	var assets []struct {
		Binary struct {
			Package struct {
				Checksum string `json:"checksum"`
				Link     string `json:"link"`
				Name     string `json:"name"`
			} `json:"package"`
		} `json:"binary"`
		ReleaseName string `json:"release_name"`
		Version     struct {
			Semver string `json:"semver"`
		} `json:"version"`
	}
	url := fmt.Sprintf(adoptiumURL, major)
	if err := getJSON(url, &assets); err != nil {
		return artifact{}, fmt.Errorf("não foi possível consultar as versões do Java: %w", err)
	}
	if len(assets) == 0 {
		return artifact{}, fmt.Errorf("não existe Java Temurin %d para Windows x64", major)
	}
	asset := assets[0]
	version := asset.Version.Semver
	if version == "" {
		version = asset.ReleaseName
	}
	if asset.Binary.Package.Link == "" || asset.Binary.Package.Checksum == "" || asset.Binary.Package.Name == "" {
		return artifact{}, errors.New("resposta incompleta da API do Eclipse Adoptium")
	}
	return artifact{
		Version:  version,
		URL:      asset.Binary.Package.Link,
		Checksum: asset.Binary.Package.Checksum,
		Name:     asset.Binary.Package.Name,
	}, nil
}

func getJSON(url string, target any) error {
	data, err := getText(url)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), target)
}

func getText(url string) (string, error) {
	if !strings.HasPrefix(url, "https://") {
		return "", errors.New("somente HTTPS é permitido")
	}
	client := http.Client{Timeout: 30 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.Request.URL.Scheme != "https" {
		return "", errors.New("redirecionamento para protocolo inseguro bloqueado")
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	return string(data), err
}

func installArtifact(root string, item artifact) error {
	if !strings.HasPrefix(item.URL, "https://") {
		return errors.New("somente downloads HTTPS são permitidos")
	}
	if item.Name == "" || filepath.Base(item.Name) != item.Name {
		return errors.New("nome de arquivo inválido na resposta remota")
	}
	if _, err := exec.LookPath("curl.exe"); err != nil {
		return errors.New("curl.exe não foi encontrado no PATH")
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	temp, err := os.MkdirTemp(root, ".vnvj-download-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)

	archive := filepath.Join(temp, item.Name)
	fmt.Printf("Baixando %s...\n", item.Name)
	command := exec.Command("curl.exe", "--fail", "--location", "--proto", "=https", "--proto-redir", "=https", "--progress-bar", "--output", archive, item.URL)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("falha no download: %w", err)
	}
	if err := validateSHA256(archive, item.Checksum); err != nil {
		return err
	}

	unpacked := filepath.Join(temp, "unpacked")
	if err := extractZIP(archive, unpacked); err != nil {
		return fmt.Errorf("falha ao extrair o arquivo: %w", err)
	}
	source, err := singleExtractedDirectory(unpacked)
	if err != nil {
		return err
	}
	targetName := item.TargetName
	if targetName == "" {
		targetName = filepath.Base(source)
	}
	target := filepath.Join(root, targetName)
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("não foi possível instalar em %s: %w", target, err)
	}
	fmt.Printf("Instalado em %s.\n", target)
	return nil
}

func validateSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("checksum SHA-256 inválido: esperado %s, obtido %s", expected, actual)
	}
	return nil
}

func extractZIP(archive, destination string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	for _, entry := range reader.File {
		name := filepath.FromSlash(entry.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("caminho inseguro no ZIP: %s", entry.Name)
		}
		target := filepath.Join(destination, name)
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("link simbólico não permitido no ZIP: %s", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		destinationFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, entry.Mode().Perm())
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(destinationFile, source)
		closeErr := destinationFile.Close()
		source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func singleExtractedDirectory(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return "", errors.New("o ZIP não contém um único diretório raiz")
	}
	return filepath.Join(root, entries[0].Name()), nil
}

func checksumFor(content, name string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == name {
			return fields[0]
		}
	}
	return ""
}
