package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unsafe"
)

const (
	profileStart = "# >>> vnvj >>>"
	profileEnd   = "# <<< vnvj <<<"
)

func installShellIntegration() error {
	bin := filepath.Join(filepath.Dir(configFile()), "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		return err
	}
	target := filepath.Join(bin, "vnvj.exe")
	current, err := os.Executable()
	if err != nil {
		return err
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return err
	}
	if !samePath(current, target) {
		data, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		if err := atomicWrite(target, data, 0755); err != nil {
			return fmt.Errorf("não foi possível instalar o executável: %w", err)
		}
	}

	for _, tool := range []string{"vn", "vj"} {
		content := cmdWrapper(tool)
		if err := atomicWrite(filepath.Join(bin, tool+".cmd"), []byte(content), 0644); err != nil {
			return err
		}
	}
	if err := atomicWrite(filepath.Join(bin, "vnvj-init.cmd"), []byte(cmdInitWrapper()), 0644); err != nil {
		return err
	}
	if err := ensureUserPath(bin); err != nil {
		return err
	}
	if err := installPowerShellProfiles(); err != nil {
		return err
	}
	if err := installCmdAutoRun(); err != nil {
		return err
	}
	broadcastEnvironmentChange()
	return nil
}

func cmdWrapper(tool string) string {
	runtime := "node"
	if tool == "vj" {
		runtime = "java"
	}
	return "@echo off\r\n" +
		"set \"_VNVJ_SCRIPT=%TEMP%\\vnvj-%RANDOM%-%RANDOM%.cmd\"\r\n" +
		"set \"VNVJ_SHELL=cmd\"\r\n" +
		"set \"VNVJ_SCRIPT=%_VNVJ_SCRIPT%\"\r\n" +
		"\"%~dp0vnvj.exe\" " + runtime + " %*\r\n" +
		"set \"_VNVJ_EXIT=%ERRORLEVEL%\"\r\n" +
		"if \"%_VNVJ_EXIT%\"==\"0\" if exist \"%_VNVJ_SCRIPT%\" call \"%_VNVJ_SCRIPT%\"\r\n" +
		"if exist \"%_VNVJ_SCRIPT%\" del /q \"%_VNVJ_SCRIPT%\"\r\n" +
		"set \"VNVJ_SCRIPT=\"\r\n" +
		"set \"VNVJ_SHELL=\"\r\n" +
		"set \"_VNVJ_SCRIPT=\"\r\n" +
		"exit /b %_VNVJ_EXIT%\r\n"
}

func cmdInitWrapper() string {
	return "@echo off\r\n" +
		"set \"_VNVJ_INIT=%TEMP%\\vnvj-init-%RANDOM%-%RANDOM%.cmd\"\r\n" +
		"set \"VNVJ_SHELL=cmd\"\r\n" +
		"set \"VNVJ_SCRIPT=%_VNVJ_INIT%\"\r\n" +
		"\"%~dp0vnvj.exe\" init cmd\r\n" +
		"if \"%ERRORLEVEL%\"==\"0\" if exist \"%_VNVJ_INIT%\" call \"%_VNVJ_INIT%\"\r\n" +
		"if exist \"%_VNVJ_INIT%\" del /q \"%_VNVJ_INIT%\"\r\n" +
		"set \"VNVJ_SCRIPT=\"\r\n" +
		"set \"VNVJ_SHELL=\"\r\n" +
		"set \"_VNVJ_INIT=\"\r\n"
}

func powerShellBlock() string {
	return profileStart + `
function global:Invoke-VnvjCommand {
    param([string]$Runtime, [object[]]$RuntimeArgs)
    $vnvjExe = Join-Path $env:LOCALAPPDATA 'vnvj\bin\vnvj.exe'
    $vnvjScript = Join-Path ([IO.Path]::GetTempPath()) ('vnvj-' + [Guid]::NewGuid().ToString('N') + '.ps1')
    try {
        $env:VNVJ_SHELL = 'powershell'
        $env:VNVJ_SCRIPT = $vnvjScript
        & $vnvjExe $Runtime @RuntimeArgs
        $vnvjExit = $LASTEXITCODE
        if ($vnvjExit -eq 0 -and (Get-Item $vnvjScript).Length -gt 0) { . $vnvjScript }
    } finally {
        Remove-Item Env:VNVJ_SHELL, Env:VNVJ_SCRIPT -ErrorAction SilentlyContinue
        Remove-Item $vnvjScript -ErrorAction SilentlyContinue
    }
}
Invoke-VnvjCommand 'init' @('powershell')
function global:vn { Invoke-VnvjCommand 'node' $args }
function global:vj { Invoke-VnvjCommand 'java' $args }
` + profileEnd
}

func installPowerShellProfiles() error {
	commands := [][]string{{"powershell.exe", "-NoProfile", "-Command", "$PROFILE.CurrentUserAllHosts"}}
	if _, err := exec.LookPath("pwsh.exe"); err == nil {
		commands = append(commands, []string{"pwsh.exe", "-NoProfile", "-Command", "$PROFILE.CurrentUserAllHosts"})
	}
	seen := map[string]bool{}
	for _, command := range commands {
		output, err := exec.Command(command[0], command[1:]...).Output()
		if err != nil {
			return fmt.Errorf("não foi possível localizar o perfil do PowerShell: %w", err)
		}
		path := strings.TrimSpace(string(output))
		if path == "" || seen[strings.ToLower(path)] {
			continue
		}
		seen[strings.ToLower(path)] = true
		var current string
		data, err := os.ReadFile(path)
		if err == nil {
			current = string(data)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		updated := upsertBlock(current, powerShellBlock())
		if err := atomicWrite(path, []byte(updated), 0644); err != nil {
			return fmt.Errorf("não foi possível atualizar %s: %w", path, err)
		}
	}
	return nil
}

func upsertBlock(content, block string) string {
	block = strings.ReplaceAll(strings.ReplaceAll(block, "\r\n", "\n"), "\n", "\r\n")
	start := strings.Index(content, profileStart)
	end := strings.Index(content, profileEnd)
	if start >= 0 && end >= start {
		end += len(profileEnd)
		return content[:start] + block + content[end:]
	}
	content = strings.TrimRight(content, "\r\n")
	if content != "" {
		content += "\r\n\r\n"
	}
	return content + block + "\r\n"
}

func installCmdAutoRun() error {
	const key = `HKCU\Software\Microsoft\Command Processor`
	current, err := queryOptionalRegistryValue(key, "AutoRun")
	if err != nil {
		return err
	}
	const hook = `call "%LOCALAPPDATA%\vnvj\bin\vnvj-init.cmd"`
	if strings.Contains(strings.ToLower(current), "vnvj-init.cmd") {
		return nil
	}
	if strings.TrimSpace(current) != "" {
		current += " & "
	}
	return setRegistryValue(key, "AutoRun", current+hook, true)
}

func ensureUserPath(bin string) error {
	const key = `HKCU\Environment`
	current, err := queryOptionalRegistryValue(key, "Path")
	if err != nil {
		return err
	}
	wanted := []string{bin, `%NODE_HOME%`, `%JAVA_HOME%\bin`}
	parts := strings.Split(current, ";")
	for i := len(wanted) - 1; i >= 0; i-- {
		if !pathEntryExists(parts, wanted[i]) {
			parts = append([]string{wanted[i]}, parts...)
		}
	}
	return setRegistryValue(key, "Path", strings.Trim(strings.Join(parts, ";"), ";"), true)
}

func pathEntryExists(entries []string, wanted string) bool {
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry), wanted) {
			return true
		}
	}
	return false
}

func setUserEnvironment(name, value string, expandable bool) error {
	return setRegistryValue(`HKCU\Environment`, name, value, expandable)
}

func setRegistryValue(key, name, value string, expandable bool) error {
	kind := "REG_SZ"
	if expandable {
		kind = "REG_EXPAND_SZ"
	}
	command := exec.Command("reg.exe", "add", key, "/v", name, "/t", kind, "/d", value, "/f")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("falha ao atualizar %s: %s", name, strings.TrimSpace(string(output)))
	}
	return nil
}

func queryRegistryValue(key, name string) (string, error) {
	output, err := exec.Command("reg.exe", "query", key, "/v", name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("falha ao consultar %s: %s", name, strings.TrimSpace(string(output)))
	}
	pattern := regexp.MustCompile(`(?mi)^\s*` + regexp.QuoteMeta(name) + `\s+REG_\w+\s+(.*)$`)
	match := pattern.FindStringSubmatch(string(output))
	if len(match) != 2 {
		return "", fmt.Errorf("valor %s não encontrado", name)
	}
	return strings.TrimSpace(match[1]), nil
}

func queryOptionalRegistryValue(key, name string) (string, error) {
	value, err := queryRegistryValue(key, name)
	if err == nil {
		return value, nil
	}
	output, keyErr := exec.Command("reg.exe", "query", key).CombinedOutput()
	if keyErr != nil {
		message := strings.ToLower(string(output))
		if strings.Contains(message, "localizar") || strings.Contains(message, "unable to find") {
			return "", nil
		}
		return "", err
	}
	return "", nil
}

func writeEnvironmentScript(shell, nodePath, javaPath string) error {
	if shell != "powershell" && shell != "cmd" {
		return fmt.Errorf("shell não suportado: %s", shell)
	}
	javaBin := ""
	if javaHome := os.Getenv("JAVA_HOME"); javaHome != "" {
		javaBin = filepath.Join(javaHome, "bin")
	}
	basePath := pathWithout(os.Getenv("PATH"), os.Getenv("NODE_HOME"), javaBin, `%NODE_HOME%`, `%JAVA_HOME%\bin`)
	var script string
	if shell == "powershell" {
		if nodePath != "" {
			script += "$env:NODE_HOME = '" + strings.ReplaceAll(nodePath, "'", "''") + "'\r\n"
		}
		if javaPath != "" {
			script += "$env:JAVA_HOME = '" + strings.ReplaceAll(javaPath, "'", "''") + "'\r\n"
		}
		script += "$vnvjPaths = @()\r\n" +
			"if ($env:NODE_HOME) { $vnvjPaths += $env:NODE_HOME }\r\n" +
			"if ($env:JAVA_HOME) { $vnvjPaths += (Join-Path $env:JAVA_HOME 'bin') }\r\n" +
			"$env:PATH = (($vnvjPaths + @('" + strings.ReplaceAll(basePath, "'", "''") + "')) -join ';')\r\n" +
			"Remove-Variable vnvjPaths -ErrorAction SilentlyContinue\r\n"
	} else {
		if nodePath != "" {
			script += "set \"NODE_HOME=" + nodePath + "\"\r\n"
		}
		if javaPath != "" {
			script += "set \"JAVA_HOME=" + javaPath + "\"\r\n"
		}
		script += "set \"VNVJ_PATH_PREFIX=\"\r\n" +
			"if defined JAVA_HOME set \"VNVJ_PATH_PREFIX=%JAVA_HOME%\\bin;%VNVJ_PATH_PREFIX%\"\r\n" +
			"if defined NODE_HOME set \"VNVJ_PATH_PREFIX=%NODE_HOME%;%VNVJ_PATH_PREFIX%\"\r\n" +
			"set \"PATH=%VNVJ_PATH_PREFIX%" + basePath + "\"\r\n" +
			"set \"VNVJ_PATH_PREFIX=\"\r\n"
	}

	target := os.Getenv("VNVJ_SCRIPT")
	if target == "" {
		fmt.Print(script)
		return nil
	}
	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if !withinDirectory(tempRoot, target) {
		return errors.New("o script de sessão precisa estar no diretório temporário")
	}
	return os.WriteFile(target, []byte(script), 0600)
}

func withinDirectory(root, path string) bool {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false
	}
	parentInfo, err := os.Stat(filepath.Dir(path))
	return err == nil && os.SameFile(rootInfo, parentInfo)
}

func pathWithout(path string, removals ...string) string {
	parts := strings.Split(path, ";")
	kept := parts[:0]
	for _, part := range parts {
		remove := false
		for _, candidate := range removals {
			if candidate != "" && samePath(part, candidate) {
				remove = true
				break
			}
		}
		if !remove && part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, ";")
}

func replaceFile(source, target string) error {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	moveFileEx := kernel32.NewProc("MoveFileExW")
	sourcePtr, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	const moveFileReplaceExisting = 0x1
	result, _, callErr := moveFileEx.Call(uintptr(unsafe.Pointer(sourcePtr)), uintptr(unsafe.Pointer(targetPtr)), moveFileReplaceExisting)
	if result == 0 {
		return callErr
	}
	return nil
}

func broadcastEnvironmentChange() {
	user32 := syscall.NewLazyDLL("user32.dll")
	send := user32.NewProc("SendMessageTimeoutW")
	name, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001a
		smtoAbortIfHung = 0x0002
	)
	var result uintptr
	_, _, _ = send.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(name)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}
