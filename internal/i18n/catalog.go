package i18n

import "fmt"

// Key identifies a message.
type Key string

// Message keys used by the CLI.
const (
	ProfileAdded       Key = "profile.added"
	ProfileRemoved     Key = "profile.removed"
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
)

var catalogEN = map[Key]string{
	ProfileAdded:       "Profile %q added.",
	ProfileRemoved:     "Profile %q removed.",
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
}

var catalogES = map[Key]string{
	ProfileAdded:       "Perfil %q agregado.",
	ProfileRemoved:     "Perfil %q eliminado.",
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
