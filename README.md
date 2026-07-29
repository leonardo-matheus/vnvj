# vnvj

[![Build](https://github.com/leonardo-matheus/vnvj/actions/workflows/build.yml/badge.svg)](https://github.com/leonardo-matheus/vnvj/actions/workflows/build.yml)
[![Release](https://img.shields.io/github/v/release/leonardo-matheus/vnvj)](https://github.com/leonardo-matheus/vnvj/releases/latest)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Plataforma](https://img.shields.io/badge/Windows-amd64-0078D4?logo=windows)](https://www.microsoft.com/windows)

Gerenciador de versões do **Node.js** e do **Java** para Windows. Selecione uma versão informando apenas o número principal, altere a sessão atual ou defina o padrão do usuário — sem privilégios administrativos.

```powershell
vn use 18
vj default 25
```

## Principais recursos

- comandos curtos e separados: `vn` para Node.js e `vj` para Java;
- seleção por versão principal: `6`, `8`, `18`, `21`, `25`;
- escolha automática da instalação local mais recente daquele major;
- versão temporária para a sessão atual com `use`;
- versão persistente para novos terminais com `default`;
- suporte a Windows PowerShell 5.1, PowerShell 7 e CMD;
- configuração completa no escopo do usuário, sem administrador;
- instalação assistida de versões ausentes;
- downloads oficiais do Node.js e Eclipse Temurin, com validação SHA-256.

## Instalação

### Usando uma Release

1. Abra a [Release mais recente](https://github.com/leonardo-matheus/vnvj/releases/latest).
2. Baixe `vnvj.exe` e, opcionalmente, confira o hash em `SHA256SUMS.txt`.
3. Execute:

```powershell
.\vnvj.exe setup
```

O assistente solicitará os diretórios que contêm as versões do Node.js e do JDK. O `setup` instala o binário e os comandos no perfil do usuário. Depois, abra um novo terminal.

### Compilando localmente

Requer Go 1.26 ou superior:

```powershell
git clone https://github.com/leonardo-matheus/vnvj.git
cd vnvj
go build -buildvcs=false -trimpath -o vnvj.exe .
.\vnvj.exe setup
```

Também é possível configurar sem perguntas:

```powershell
.\vnvj.exe setup `
  --node "$HOME\Software\Nodejs" `
  --java "$HOME\Software\JDK"
```

## Uso

### Node.js

```powershell
vn                 # lista as versões instaladas
vn use 18          # usa a versão 18 nesta sessão
vn default 22      # define a versão 22 como padrão
```

### Java

```powershell
vj                 # lista as versões instaladas
vj use 8           # usa Java 8 nesta sessão
vj default 25      # define Java 25 como padrão
```

Quando há mais de uma instalação do mesmo major, a versão mais recente é selecionada. Por exemplo, `vn use 18` escolhe `18.18.0` em vez de `18.12.0`, e `vj use 8` escolhe `8u412` em vez de `8u201`.

## Versões ausentes

Se o major solicitado existe oficialmente, mas não está no diretório local, o vnvj apresenta a versão encontrada e solicita confirmação:

```text
Node.js 24 não está instalado. Instalar 24.18.0? [s/N]:
```

Após a confirmação, o arquivo é baixado por HTTPS com `curl.exe`, o SHA-256 é validado, a versão é extraída no diretório configurado e o comando original continua automaticamente.

| Runtime | Origem do download | Pacote |
|---|---|---|
| Node.js | Distribuição oficial do Node.js | Windows x64 ZIP |
| Java | Eclipse Adoptium | Temurin JDK HotSpot Windows x64 ZIP |

## Como funciona

Um executável não consegue alterar as variáveis do processo que o iniciou. Por isso, o `setup` instala wrappers mínimos para PowerShell e CMD. Esses wrappers executam o binário e aplicam `NODE_HOME`, `JAVA_HOME` e `PATH` na sessão chamadora.

O comando `default` também atualiza as variáveis de ambiente do usuário. Terminais já abertos não recebem alterações persistentes do Windows retroativamente, mas a sessão atual é atualizada pelo wrapper.

Arquivos instalados:

```text
%LOCALAPPDATA%\vnvj\
├── bin\
│   ├── vnvj.exe
│   ├── vn.cmd
│   ├── vj.cmd
│   └── vnvj-init.cmd
└── config.json
```

## Reconfiguração

Execute o `setup` novamente para trocar os diretórios ou atualizar a integração dos terminais:

```powershell
vnvj.exe setup --node "D:\Nodejs" --java "D:\JDK"
```

## Solução de problemas

### O comando `vn` ou `vj` não foi encontrado

Abra um novo terminal após o `setup`. No PowerShell, também é possível recarregar o perfil atual:

```powershell
. $PROFILE.CurrentUserAllHosts
```

### A versão não mudou

Confirme a origem do executável encontrado:

```powershell
Get-Command node
Get-Command java
node --version
java -version
```

Use `vn use <major>` e `vj use <major>` pelos comandos instalados. A chamada direta ao binário exige o runtime explícito, por exemplo `vnvj.exe node use 18`.

## Desenvolvimento

```powershell
gofmt -w *.go
go test ./...
go vet ./...
go build -buildvcs=false -trimpath -o vnvj.exe .
```

O workflow do GitHub Actions executa as mesmas verificações em Windows e publica o executável com seu checksum SHA-256 como artefato.

## Licença

Este projeto ainda não possui uma licença definida.
