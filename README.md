# claude-identity

[English](./README.en.md) · Español

CLI (`cid`) y plugin de Claude Code para mantener varias cuentas de Anthropic y cambiar entre ellas dentro de una sesión en curso, sin volver a iniciar sesión y sin perder el contexto de la conversación.

> Esta versión en español resume la referencia. La descripción completa de cada comando y de la configuración está en [README.en.md](./README.en.md).

## Por qué

Claude Code admite una sola cuenta a la vez. Cambiar de cuenta implica cerrar sesión, iniciar de nuevo y, casi siempre, reiniciar la sesión. `claude-identity` guarda las credenciales de las cuentas inactivas en el llavero del sistema operativo y reemplaza la activa en el momento, así que el mismo directorio `~/.claude` y la misma conversación siguen funcionando.

## Cómo funciona

- Se sigue usando un único directorio `~/.claude`.
- Las credenciales de las cuentas inactivas (el archivo de credenciales en Windows y Linux, el elemento `Claude Code-credentials` de Keychain en macOS, más el bloque `oauthAccount` de `~/.claude.json`) viven en el llavero del sistema: Windows Credential Manager, macOS Keychain o Linux Secret Service.
- Cuando un token expira, Claude Code lo renueva. Si eso falla, ejecuta `cid relogin <nombre>` (o `/account relogin <nombre>` en Claude Code).
- `cid` nunca llama por su cuenta a los endpoints OAuth de Anthropic.

Plataformas: Windows, macOS y Linux. Idiomas de la interfaz: inglés y español.

## Comandos del plugin

El repositorio también es un marketplace de plugins de Claude Code. El plugin ofrece:

- `/account` para listar cuentas.
- `/account 2` o `/account <nombre>` para cambiar.
- `/account add <nombre>` para iniciar sesión en el navegador y guardar la cuenta como perfil.
- `/account relogin <n|nombre>` para renovar el inicio de sesión de un perfil.
- `/account add <nombre> --current` para capturar el inicio de sesión activo (ejecuta `/login` antes).
- `/account config` para configurar idioma y opciones.
- `/account auto ...` para configurar el cambio automático.

Las mismas operaciones están disponibles desde el binario `cid`.

## Instalación

### 1. Instalar `cid`

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/nicoarbelaez/claude-identity/main/scripts/install.ps1 | iex
```

macOS y Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/nicoarbelaez/claude-identity/main/scripts/install.sh | sh
```

O con Go:

```sh
go install github.com/nicoarbelaez/claude-identity/cmd/cid@latest
```

Los scripts descargan el archivo de la versión para tu sistema y arquitectura, verifican su SHA-256 contra `checksums.txt` (y abortan si no coincide) e instalan `cid` en `%LOCALAPPDATA%\Programs\cid` (Windows) o `~/.local/bin` (macOS y Linux). Si tienes GitHub CLI, también ejecutan `gh attestation verify`.

- Windows: el directorio de instalación se agrega al `PATH` del usuario. Con `CID_NO_MODIFY_PATH=1` no se modifica.
- macOS y Linux: si el directorio no está en el `PATH`, el script imprime la línea que debes agregar a tu shell.
- Fija una versión con `CID_VERSION=<versión>` y cambia el directorio con `CID_INSTALL_DIR`.

Detalles de publicación: [docs/RELEASING.md](docs/RELEASING.md).

### 2. Verificar

Abre una terminal nueva y ejecuta:

```sh
cid version
```

### 3. Instalar el plugin de Claude Code

En Claude Code (2.1.275 o posterior):

```
/plugin marketplace add nicoarbelaez/claude-identity
/plugin install account-switch@nicoarbelaez-tools
```

Reinicia Claude Code para que detecte `cid` en el `PATH`. Los comandos del plugin llevan el prefijo `/account-switch:account`; `/account` funciona mientras el nombre no sea ambiguo.

### 4. Primer uso

1. Ejecuta `cid add a` (o `/account add a` en Claude Code) e inicia sesión con tu primera cuenta en el navegador.
2. Ejecuta `cid add b` e inicia sesión con la segunda.
3. Cambia con `/account 1` o `/account b`.

Para probar el plugin desde una copia local sin instalarlo: `claude --plugin-dir ./plugin`.

## Actualizar y desinstalar

```sh
cid update       # actualiza el binario (con checksum) y el plugin
cid uninstall    # lista lo que elimina y pide confirmación
```

`cid update` descarga la última versión, la verifica contra `checksums.txt`, reemplaza el binario y luego actualiza el marketplace y el plugin con `claude plugin`. Reinicia Claude Code o ejecuta `/reload-plugins`. Si el checksum no coincide o la descarga falla, sale con código `1` y deja el binario intacto.

`cid uninstall` elimina las credenciales guardadas de cada perfil (llavero), el archivo de perfiles y la carpeta de configuración vacía; nunca toca el inicio de sesión propio de Claude Code. En Windows no puede borrar su propio binario en ejecución: imprime la ruta y el comando para hacerlo a mano. Después quita el plugin con `/plugin uninstall account-switch@nicoarbelaez-tools` y `/plugin marketplace remove nicoarbelaez-tools`.

## Uso

| Comando | Qué hace |
| --- | --- |
| `cid` / `cid list [--json]` | Lista perfiles con barras de uso de 5 h y de la semana |
| `cid <n\|nombre>` / `cid switch` | Cambia a un perfil. Las sesiones de Remote Control se desconectan |
| `cid add <nombre> [--force] [--console] [--sso]` | Inicia sesión en un directorio aislado y guarda la cuenta |
| `cid add <nombre> --current` | Guarda el inicio de sesión activo como perfil |
| `cid remove <n\|nombre>` | Elimina un perfil y su entrada del llavero |
| `cid relogin <n\|nombre>` | Renueva el inicio de sesión de un perfil |
| `cid current [--json]` | Muestra el perfil activo |
| `cid config [get\|set\|unset]` | Ver y cambiar ajustes |
| `cid statusline [install\|uninstall]` | Registra el uso del plan desde la línea de estado |
| `cid auto ...` | Cambio automático de cuenta |
| `cid update` / `cid version` | Actualizar y ver la versión |

Salidas `--json` estables para scripts. Códigos de salida: `0` éxito, `1` error de ejecución, `2` error de uso.

### Agregar cuentas

`cid add <nombre>` ejecuta el inicio de sesión de Claude Code (`claude auth login`) en un directorio de configuración temporal y privado, de modo que el inicio de sesión actual y el perfil activo no se tocan. Guarda la cuenta como perfil y borra el directorio temporal, también ante un error o Ctrl+C. `claude` debe estar en el `PATH`. En macOS el inicio de sesión aislado aún no está soportado: usa `/login` y luego `cid add <nombre> --current`.

### Respuestas instantáneas

El plugin registra un hook `UserPromptExpansion` que ejecuta `cid hook` al escribir `/account`. La salida se muestra tal cual, sin turno del modelo y sin gasto de tokens. Los argumentos nunca pasan por un shell: se ejecuta `cid` directamente y se rechaza cualquier token que no sea un nombre de perfil, un número o `--json`.

### Uso por cuenta

`cid list` muestra cuánto del plan se ha usado en cada cuenta, con barras para 5 horas y para la semana y el tiempo hasta el reinicio. Los datos salen del campo `rate_limits` que Claude Code entrega a su línea de estado (solo en cuentas Pro y Max, después de la primera respuesta). Para activarlo:

```sh
cid statusline install     # envuelve la línea de estado actual
cid statusline uninstall   # la restaura byte a byte
```

### Cambio automático

`cid auto` cambia de cuenta cuando la activa se acerca a su límite (90 % en 5 h y 95 % en la semana por defecto). Un hook `UserPromptSubmit` aplica el cambio justo antes de enviar tu siguiente mensaje, nunca a mitad de una respuesta. Elige primero la cuenta cuya ventana semanal se reinicia antes; hay una cuenta principal y una de reserva, y reglas anti-oscilación (5 puntos de margen y 5 minutos entre cambios).

```sh
cid statusline install
cid auto add 1
cid auto principal trabajo
cid auto reserve respaldo
cid auto on
```

Supone que esta máquina es el único lugar donde se usan esas cuentas: el uso desde otros equipos no se ve.

### Cambio seguro

- Espera los bloqueos de credenciales de Claude Code antes de leer o escribir un inicio de sesión.
- Se niega a guardar un inicio de sesión vacío o desconocido.
- Si la verificación falla tras un cambio, restaura el inicio de sesión anterior.
- Un token cuenta como expirado 5 minutos antes de su expiración real.

### Configuración e idioma

```sh
cid config set language es
```

Un ajuste se resuelve en este orden: variable de entorno, `config.json`, valor por defecto. `language` admite `auto`, `en` y `es` (variable `CID_LANG`). Con `auto`, el idioma se elige con `CID_LANG`, el ajuste `language` de Claude Code, `LC_ALL`/`LC_MESSAGES`/`LANG`, el idioma del sistema y, por último, inglés.

Los archivos de configuración se validan con un JSON Schema embebido; si se rompen a mano, `cid` los mueve a `<archivo>.corrupt-<fecha>`, restaura la copia `.bak` y avisa. La recuperación nunca borra entradas del llavero.

## Desarrollo

Requisitos: Go 1.26.0.

```sh
go build ./cmd/cid
go test -race ./...
```

La integración continua corre `gofmt`, `go vet` y `go test -race ./...` en Ubuntu, macOS y Windows, además de CodeQL. Las versiones las gestiona release-please y GoReleaser compila para Windows, macOS y Linux en amd64 y arm64. Guía: [CONTRIBUTING.md](CONTRIBUTING.md) y [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

Arquitectura con diagramas: [docs/architecture.md](docs/architecture.md).

## Seguridad

Esta herramienta maneja tokens OAuth. Los guarda solo en el llavero del sistema y nunca los envía a ningún lado. Reporta vulnerabilidades en privado: ver [SECURITY.md](SECURITY.md).

## Licencia

[MIT](LICENSE)
