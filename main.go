package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	NodeRoot    string `json:"node_root"`
	JavaRoot    string `json:"java_root"`
	NodeDefault string `json:"node_default,omitempty"`
	JavaDefault string `json:"java_default,omitempty"`
}

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "Erro:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("comando inválido")
	}

	name := strings.TrimSuffix(strings.ToLower(filepath.Base(args[0])), ".exe")
	if name == "vn" || name == "vj" {
		kind := nodeRuntime
		if name == "vj" {
			kind = javaRuntime
		}
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return runRuntime(kind, args[1:], &cfg)
	}

	if len(args) == 1 || args[1] == "help" || args[1] == "--help" || args[1] == "-h" {
		printHelp()
		return nil
	}

	switch strings.ToLower(args[1]) {
	case "setup":
		return runSetup(args[2:])
	case "node", "vn":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return runRuntime(nodeRuntime, args[2:], &cfg)
	case "java", "vj":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return runRuntime(javaRuntime, args[2:], &cfg)
	case "init":
		if len(args) != 3 {
			return errors.New("shell ausente no comando interno init")
		}
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return writeEnvironmentScript(args[2], cfg.NodeDefault, cfg.JavaDefault)
	default:
		return fmt.Errorf("comando desconhecido %q; use vn/vj ou vnvj node/java", args[1])
	}
}

func runRuntime(kind runtimeKind, args []string, cfg *config) error {
	action := "list"
	if len(args) > 0 {
		action = strings.ToLower(args[0])
	}
	if action == "help" || action == "--help" || action == "-h" {
		if len(args) > 1 {
			return errors.New("o comando help não recebe argumentos")
		}
		printRuntimeHelp(kind)
		return nil
	}
	if action == "list" || action == "ls" {
		if len(args) > 1 {
			return errors.New("o comando list não recebe argumentos")
		}
		return listInstallations(kind, *cfg)
	}
	if action != "use" && action != "default" {
		return fmt.Errorf("comando desconhecido %q; use %s help", action, commandName(kind))
	}
	if len(args) != 2 {
		return fmt.Errorf("uso: %s %s <versão principal>", commandName(kind), action)
	}
	if action == "use" && os.Getenv("VNVJ_SCRIPT") == "" {
		return fmt.Errorf("%s use precisa ser executado pelo comando instalado %s", commandName(kind), commandName(kind))
	}

	major, err := parseMajor(args[1])
	if err != nil {
		return err
	}
	selected, err := ensureInstalled(kind, rootFor(*cfg, kind), major)
	if err != nil {
		return err
	}

	if os.Getenv("VNVJ_SCRIPT") != "" {
		nodePath, javaPath := "", ""
		if kind == nodeRuntime {
			nodePath = selected.Path
		} else {
			javaPath = selected.Path
		}
		if err := writeEnvironmentScript(os.Getenv("VNVJ_SHELL"), nodePath, javaPath); err != nil {
			return err
		}
	}
	if action == "default" {
		if err := persistDefault(kind, selected.Path, cfg); err != nil {
			return err
		}
	}

	label := "Node.js"
	if kind == javaRuntime {
		label = "Java"
	}
	if action == "default" {
		fmt.Printf("%s %s definido como padrão.\n", label, versionString(selected.Version))
		if os.Getenv("VNVJ_SCRIPT") == "" {
			fmt.Println("Abra um novo terminal para aplicar a versão.")
		}
	} else {
		fmt.Printf("%s %s selecionado nesta sessão.\n", label, versionString(selected.Version))
	}
	return nil
}

func listInstallations(kind runtimeKind, cfg config) error {
	root := rootFor(cfg, kind)
	installations, err := discover(root, kind)
	if err != nil {
		return err
	}
	if len(installations) == 0 {
		fmt.Printf("Nenhuma versão instalada em %s.\n", root)
		return nil
	}

	active := os.Getenv("NODE_HOME")
	defaultPath := cfg.NodeDefault
	label := "Node.js"
	if kind == javaRuntime {
		active = os.Getenv("JAVA_HOME")
		defaultPath = cfg.JavaDefault
		label = "Java"
	}
	fmt.Printf("%s em %s:\n", label, root)
	for _, item := range installations {
		var marks []string
		if samePath(item.Path, active) {
			marks = append(marks, "sessão")
		}
		if samePath(item.Path, defaultPath) {
			marks = append(marks, "padrão")
		}
		mark := ""
		if len(marks) > 0 {
			mark = " [" + strings.Join(marks, ", ") + "]"
		}
		fmt.Printf("  %-12s %s%s\n", versionString(item.Version), item.Name, mark)
	}
	fmt.Printf("\nUse %s help para ver os comandos disponíveis.\n", commandName(kind))
	return nil
}

func runSetup(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	set := flag.NewFlagSet("setup", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	nodeRoot := set.String("node", "", "diretório das versões do Node.js")
	javaRoot := set.String("java", "", "diretório das versões do JDK")
	if err := set.Parse(args); err != nil {
		return fmt.Errorf("parâmetros inválidos: %w", err)
	}
	if set.NArg() != 0 {
		return errors.New("uso: vnvj setup [--node diretório] [--java diretório]")
	}

	reader := bufio.NewReader(os.Stdin)
	if *nodeRoot == "" {
		*nodeRoot, err = promptRoot(reader, "Diretório das versões do Node.js", cfg.NodeRoot)
		if err != nil {
			return err
		}
	}
	if *javaRoot == "" {
		*javaRoot, err = promptRoot(reader, "Diretório das versões do JDK", cfg.JavaRoot)
		if err != nil {
			return err
		}
	}
	cfg.NodeRoot, err = prepareRoot(*nodeRoot)
	if err != nil {
		return fmt.Errorf("diretório do Node.js inválido: %w", err)
	}
	cfg.JavaRoot, err = prepareRoot(*javaRoot)
	if err != nil {
		return fmt.Errorf("diretório do JDK inválido: %w", err)
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	if err := installShellIntegration(); err != nil {
		return err
	}
	fmt.Println("Configuração concluída. Abra um novo terminal e use vn ou vj.")
	return nil
}

func promptRoot(reader *bufio.Reader, label, current string) (string, error) {
	fmt.Printf("%s [%s]: ", label, current)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return current, nil
	}
	return strings.Trim(value, "\""), nil
}

func prepareRoot(path string) (string, error) {
	if strings.ContainsAny(path, "\r\n%") {
		return "", errors.New("o caminho não pode conter quebra de linha ou %")
	}
	absolute, err := filepath.Abs(os.ExpandEnv(path))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0755); err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("o caminho não é um diretório")
	}
	return filepath.Clean(absolute), nil
}

func loadConfig() (config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(configFile())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("configuração inválida em %s: %w", configFile(), err)
	}
	defaults := defaultConfig()
	if cfg.NodeRoot == "" {
		cfg.NodeRoot = defaults.NodeRoot
	}
	if cfg.JavaRoot == "" {
		cfg.JavaRoot = defaults.JavaRoot
	}
	return cfg, nil
}

func saveConfig(cfg config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := atomicWrite(configFile(), data, 0600); err != nil {
		return fmt.Errorf("não foi possível salvar %s: %w", configFile(), err)
	}
	return nil
}

func defaultConfig() config {
	home, _ := os.UserHomeDir()
	return config{
		NodeRoot: filepath.Join(home, "Software", "Nodejs"),
		JavaRoot: filepath.Join(home, "Software", "JDK"),
	}
}

func configFile() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	return filepath.Join(base, "vnvj", "config.json")
}

func rootFor(cfg config, kind runtimeKind) string {
	if kind == javaRuntime {
		return cfg.JavaRoot
	}
	return cfg.NodeRoot
}

func commandName(kind runtimeKind) string {
	if kind == javaRuntime {
		return "vj"
	}
	return "vn"
}

func persistDefault(kind runtimeKind, path string, cfg *config) error {
	name := "NODE_HOME"
	old := cfg.NodeDefault
	cfg.NodeDefault = path
	if kind == javaRuntime {
		name = "JAVA_HOME"
		old = cfg.JavaDefault
		cfg.JavaDefault = path
	}
	if err := setUserEnvironment(name, path, false); err != nil {
		if kind == javaRuntime {
			cfg.JavaDefault = old
		} else {
			cfg.NodeDefault = old
		}
		return err
	}
	if err := saveConfig(*cfg); err != nil {
		_ = setUserEnvironment(name, old, false)
		if kind == javaRuntime {
			cfg.JavaDefault = old
		} else {
			cfg.NodeDefault = old
		}
		return err
	}
	broadcastEnvironmentChange()
	return nil
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".vnvj-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return replaceFile(tempName, path)
}

func printRuntimeHelp(kind runtimeKind) {
	command := commandName(kind)
	label := "Node.js"
	if kind == javaRuntime {
		label = "Java"
	}
	fmt.Printf(`%s — gerenciador de versões do %s

Comandos:
  %s                 lista as versões instaladas
  %s use <versão>    usa a versão nesta sessão
  %s default <versão> define a versão padrão do usuário
  %s help            mostra esta ajuda

Informe apenas o número principal da versão, por exemplo: %s use 18.
`, command, label, command, command, command, command, command)
}

func printHelp() {
	fmt.Print(`vnvj — gerenciador de versões do Node.js e Java para Windows

Instalação:
  vnvj setup                         configura pastas e comandos globais
  vnvj setup --node DIR --java DIR   configura sem perguntas

Uso após o setup:
  vn                                 lista versões do Node.js
  vn use 24                          usa Node.js 24 nesta sessão
  vn default 18                      define Node.js 18 como padrão
  vj                                 lista versões do Java
  vj use 8                           usa Java 8 nesta sessão
  vj default 25                      define Java 25 como padrão
`)
}
