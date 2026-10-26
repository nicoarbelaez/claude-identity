package i18n

import "fmt"

// Key identifies a message.
type Key string

// Message keys used by the CLI.
const (
	ProfileAdded       Key = "profile.added"
	ProfileReplaced    Key = "profile.replaced"
	ProfileRemoved     Key = "profile.removed"
	ProfileExists      Key = "error.profile_exists"
	AccountSaved       Key = "error.account_already_saved"
	MissingName        Key = "error.missing_name"
	ProfileListHeader  Key = "profile.list_header"
	SwitchedFromTo     Key = "switch.from_to"
	SwitchedTo         Key = "switch.to"
	AlreadyActive      Key = "switch.already_active"
	UnknownProfile     Key = "error.unknown_profile"
	LiveAccountUnknown Key = "error.live_account_unknown"
	LockTimeout        Key = "error.lock_timeout"
	TokenExpired       Key = "error.token_expired"
	RemoteControlWarn  Key = "warn.remote_control_disconnected"
	RestartHint        Key = "hint.restart"
	VersionLine        Key = "version.line"

	ListEmpty          Key = "profile.list_empty"
	CurrentActive      Key = "current.active"
	CurrentNone        Key = "current.none"
	ConfigProfilesPath Key = "config.profiles_path"
	ConfigClaudeDir    Key = "config.claude_dir"
	ConfigBackend      Key = "config.backend"
	ConfigLanguage     Key = "config.language"
	BackendFile        Key = "config.backend_file"
	BackendKeychain    Key = "config.backend_keychain"
	NoLiveLogin        Key = "error.no_live_login"
	InvalidName        Key = "error.invalid_name"
	RemoveActive       Key = "error.remove_active"
	GenericError       Key = "error.generic"
	UnknownCommand     Key = "error.unknown_command"
	HookDone           Key = "hook.done"
	Usage              Key = "usage.text"

	StatusOK            Key = "status.ok"
	StatusAccessExpired Key = "status.access_expired"
	StatusNeedsLogin    Key = "status.needs_login"
	StatusUnknown       Key = "status.unknown"
	CurrentStatus       Key = "current.status"
	ReloginSteps        Key = "relogin.steps"
)

// Message keys for `cid uninstall`.
const (
	UninstallPlan        Key = "uninstall.plan"
	UninstallPlanPath    Key = "uninstall.plan_path"
	UninstallPrompt      Key = "uninstall.prompt"
	UninstallAborted     Key = "uninstall.aborted"
	UninstallProfile     Key = "uninstall.profile"
	UninstallFileRemoved Key = "uninstall.file_removed"
	UninstallPathRemoved Key = "uninstall.path_removed"
	UninstallStepFailed  Key = "uninstall.step_failed"
	UninstallDone        Key = "uninstall.done"
	UninstallIncomplete  Key = "uninstall.incomplete"
	UninstallManual      Key = "uninstall.manual"
	UninstallDeleteCmd   Key = "uninstall.delete_cmd"
)

var catalogEN = map[Key]string{
	ProfileAdded:       "Profile %q added.",
	ProfileRemoved:     "Profile %q removed.",
	ProfileReplaced:    "Profile %q replaced.",
	ProfileExists:      "Profile %q already holds %s; use --force to replace it.",
	AccountSaved:       "This account is already saved as %q.",
	MissingName:        "Missing profile name.",
	ProfileListHeader:  "Profiles:",
	SwitchedFromTo:     "Switched from %q to %q.",
	SwitchedTo:         "Switched to %q.",
	AlreadyActive:      "Profile %q is already active.",
	UnknownProfile:     "Unknown profile %q. Run `cid list` to see available profiles.",
	LiveAccountUnknown: "The logged-in account is not saved as a profile. Run `cid add <name>` to save it first.",
	LockTimeout:        "Timed out waiting for another cid process to finish. Try again in a moment.",
	TokenExpired:       "The saved login for %q has expired. Run `/login` in Claude Code, then `cid add %s` to update it.",
	RemoteControlWarn:  "Warning: switching accounts disconnects Remote Control. Reconnect it after the switch.",
	RestartHint:        "If Claude Code still shows the previous account, restart it with `claude --continue`.",
	VersionLine:        "cid %s",

	ListEmpty:          "No profiles saved yet. Log in to Claude Code, then run `cid add <name>`.",
	CurrentActive:      "Active profile: %s (%s)",
	CurrentNone:        "No active profile.",
	ConfigProfilesPath: "Profiles file: %s",
	ConfigClaudeDir:    "Claude config dir: %s",
	ConfigBackend:      "Credential backend: %s",
	ConfigLanguage:     "Language: %s",
	BackendFile:        "credentials file",
	BackendKeychain:    "macOS Keychain",
	NoLiveLogin:        "Claude Code has no login to save. Run `claude`, then `/login`, and try again.",
	InvalidName:        "Invalid profile name. Use 1-32 characters of a-z, 0-9, '.', '_' or '-', starting with a letter or digit.",
	RemoveActive:       "Cannot remove %q: it is the active profile. Switch to another profile first.",
	GenericError:       "Error: %s",
	UnknownCommand:     "Unknown command %q.",
	HookDone:           "Done.",

	StatusOK:            "ok",
	StatusAccessExpired: "token expired (auto-refresh)",
	StatusNeedsLogin:    "login required",
	StatusUnknown:       "unknown",
	CurrentStatus:       "Status: %s",
	ReloginSteps:        "1. In Claude Code run /login and sign in as %s.\n2. Run: cid add %s",

	UninstallPlan:        "This will remove:\n  - %d saved profile(s) and their stored credentials\n  - the profiles file and cid config folder: %s",
	UninstallPlanPath:    "  - %s from your user PATH",
	UninstallPrompt:      "Claude Code's own login is not touched. Continue? [y/N] ",
	UninstallAborted:     "Nothing removed.",
	UninstallProfile:     "Removed stored credentials of profile %q.",
	UninstallFileRemoved: "Removed %s.",
	UninstallPathRemoved: "Removed %s from your user PATH. Open a new terminal for it to take effect.",
	UninstallStepFailed:  "Could not remove %s: %s",
	UninstallDone:        "cid data removed. Claude Code stays logged in with its current account.",
	UninstallIncomplete:  "Uninstall finished with errors; see the messages above.",
	UninstallManual:      "To finish, remove the binary and the plugin yourself:\n  1. Binary: %s\n     %s\n  2. In Claude Code run:\n     /plugin uninstall account-switch@nicoarbelaez-tools\n     /plugin marketplace remove nicoarbelaez-tools",
	UninstallDeleteCmd:   "Delete it with: %s",
	Usage: `Usage:
  cid                       List profiles
  cid list [--json]         List profiles
  cid <n|name>              Switch to a profile
  cid switch <n|name>       Switch to a profile
  cid add <name> [--force]  Save the current Claude Code login as a profile
  cid remove <n|name>       Remove a profile (not the active one)
  cid relogin <n|name>      Show the steps to refresh a profile's login
  cid current [--json]      Show the active profile
  cid config                Show config locations and language
  cid uninstall [--yes]     Remove all cid data (profiles, stored credentials, PATH entry)
  cid version               Show the version
  cid help                  Show this help`,
}

var catalogES = map[Key]string{
	ProfileAdded:       "Perfil %q agregado.",
	ProfileRemoved:     "Perfil %q eliminado.",
	ProfileReplaced:    "Perfil %q reemplazado.",
	ProfileExists:      "El perfil %q ya contiene %s; usa --force para reemplazarlo.",
	AccountSaved:       "Esta cuenta ya está guardada como %q.",
	MissingName:        "Falta el nombre del perfil.",
	ProfileListHeader:  "Perfiles:",
	SwitchedFromTo:     "Cambio de %q a %q.",
	SwitchedTo:         "Cambio a %q.",
	AlreadyActive:      "El perfil %q ya está activo.",
	UnknownProfile:     "Perfil %q desconocido. Ejecuta `cid list` para ver los perfiles disponibles.",
	LiveAccountUnknown: "La cuenta con sesión iniciada no está guardada como perfil. Ejecuta `cid add <nombre>` para guardarla primero.",
	LockTimeout:        "Se agotó el tiempo de espera de otro proceso de cid. Inténtalo de nuevo en un momento.",
	TokenExpired:       "El inicio de sesión guardado de %q ha caducado. Ejecuta `/login` en Claude Code y luego `cid add %s` para actualizarlo.",
	RemoteControlWarn:  "Advertencia: cambiar de cuenta desconecta Remote Control. Vuelve a conectarlo después del cambio.",
	RestartHint:        "Si Claude Code aún muestra la cuenta anterior, reinícialo con `claude --continue`.",
	VersionLine:        "cid %s",

	ListEmpty:          "Aún no hay perfiles guardados. Inicia sesión en Claude Code y ejecuta `cid add <nombre>`.",
	CurrentActive:      "Perfil activo: %s (%s)",
	CurrentNone:        "No hay un perfil activo.",
	ConfigProfilesPath: "Archivo de perfiles: %s",
	ConfigClaudeDir:    "Directorio de configuración de Claude: %s",
	ConfigBackend:      "Almacén de credenciales: %s",
	ConfigLanguage:     "Idioma: %s",
	BackendFile:        "archivo de credenciales",
	BackendKeychain:    "Llavero de macOS",
	NoLiveLogin:        "Claude Code no tiene ninguna sesión que guardar. Ejecuta `claude`, luego `/login` y vuelve a intentarlo.",
	InvalidName:        "Nombre de perfil no válido. Usa de 1 a 32 caracteres entre a-z, 0-9, '.', '_' o '-', empezando por una letra o un dígito.",
	RemoveActive:       "No se puede eliminar %q: es el perfil activo. Cambia primero a otro perfil.",
	GenericError:       "Error: %s",
	UnknownCommand:     "Comando desconocido %q.",
	HookDone:           "Listo.",

	StatusOK:            "ok",
	StatusAccessExpired: "token vencido (se renueva solo)",
	StatusNeedsLogin:    "requiere iniciar sesión",
	StatusUnknown:       "desconocido",
	CurrentStatus:       "Estado: %s",
	ReloginSteps:        "1. En Claude Code ejecuta /login e inicia sesión como %s.\n2. Ejecuta: cid add %s",

	UninstallPlan:        "Se eliminará:\n  - %d perfil(es) guardado(s) y sus credenciales almacenadas\n  - el archivo de perfiles y la carpeta de configuración de cid: %s",
	UninstallPlanPath:    "  - %s del PATH de tu usuario",
	UninstallPrompt:      "La sesión propia de Claude Code no se toca. ¿Continuar? [s/N] ",
	UninstallAborted:     "No se eliminó nada.",
	UninstallProfile:     "Se eliminaron las credenciales almacenadas del perfil %q.",
	UninstallFileRemoved: "Se eliminó %s.",
	UninstallPathRemoved: "Se quitó %s del PATH de tu usuario. Abre una terminal nueva para que surta efecto.",
	UninstallStepFailed:  "No se pudo eliminar %s: %s",
	UninstallDone:        "Datos de cid eliminados. Claude Code sigue con la sesión de su cuenta actual.",
	UninstallIncomplete:  "La desinstalación terminó con errores; revisa los mensajes anteriores.",
	UninstallManual:      "Para terminar, elimina tú mismo el binario y el plugin:\n  1. Binario: %s\n     %s\n  2. En Claude Code ejecuta:\n     /plugin uninstall account-switch@nicoarbelaez-tools\n     /plugin marketplace remove nicoarbelaez-tools",
	UninstallDeleteCmd:   "Elimínalo con: %s",
	Usage: `Uso:
  cid                       Lista los perfiles
  cid list [--json]         Lista los perfiles
  cid <n|nombre>            Cambia a un perfil
  cid switch <n|nombre>     Cambia a un perfil
  cid add <nombre> [--force] Guarda la sesión actual de Claude Code como perfil
  cid remove <n|nombre>     Elimina un perfil (no el activo)
  cid relogin <n|nombre>    Muestra los pasos para renovar la sesión de un perfil
  cid current [--json]      Muestra el perfil activo
  cid config                Muestra las rutas de configuración y el idioma
  cid uninstall [--yes]     Elimina todos los datos de cid (perfiles, credenciales guardadas, entrada del PATH)
  cid version               Muestra la versión
  cid help                  Muestra esta ayuda`,
}

// T returns the message for key in lang, formatted with args. A key missing
// from the language catalog falls back to English, then to the key itself.
func T(lang Lang, key Key, args ...any) string {
	msg, ok := "", false
	if lang == ES {
		msg, ok = catalogES[key]
	}
	if !ok {
		msg, ok = catalogEN[key]
	}
	if !ok {
		return string(key)
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}
