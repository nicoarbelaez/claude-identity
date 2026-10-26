package i18n

// Message keys for `cid config` and for file recovery notices.
const (
	SettingLanguage Key = "setting.language"

	ConfigFilePath     Key = "config.file_path"
	ConfigSourceEnv    Key = "config.source_env"
	ConfigSourceFile   Key = "config.source_file"
	ConfigSourceDef    Key = "config.source_default"
	ConfigUnknownKey   Key = "config.error_unknown_key"
	ConfigInvalidValue Key = "config.error_invalid_value"
	ConfigSetDone      Key = "config.set_done"
	ConfigUnsetDone    Key = "config.unset_done"

	NoticeReset         Key = "notice.file_reset"
	NoticeRestored      Key = "notice.file_restored"
	NoticeActiveCleared Key = "notice.active_cleared"
	NoticeUnknownKey    Key = "notice.unknown_key"
	NoticeInvalidValue  Key = "notice.invalid_value"
)

var configEN = map[Key]string{
	SettingLanguage: "Interface language (auto detects it from your system).",

	ConfigFilePath:     "Settings file: %s",
	ConfigSourceEnv:    "environment",
	ConfigSourceFile:   "file",
	ConfigSourceDef:    "default",
	ConfigUnknownKey:   "Unknown setting %q. Available: %s.",
	ConfigInvalidValue: "Invalid value %q for %s. Allowed: %s.",
	ConfigSetDone:      "%s = %s",
	ConfigUnsetDone:    "%s restored to its default: %s",

	NoticeReset:         "Warning: %s could not be read. It was moved to %s and cid started with an empty one.",
	NoticeRestored:      "Warning: %s could not be read. It was moved to %s and restored from the last good backup.",
	NoticeActiveCleared: "Warning: the active profile marker pointed at a missing profile (%s) and was cleared.",
	NoticeUnknownKey:    "Warning: %s has an unknown setting %q, ignoring it.",
	NoticeInvalidValue:  "Warning: %s has an invalid value for %q, using the default.",
}

var configES = map[Key]string{
	SettingLanguage: "Idioma de la interfaz (auto lo detecta desde el sistema).",

	ConfigFilePath:     "Archivo de ajustes: %s",
	ConfigSourceEnv:    "entorno",
	ConfigSourceFile:   "archivo",
	ConfigSourceDef:    "predeterminado",
	ConfigUnknownKey:   "Ajuste desconocido %q. Disponibles: %s.",
	ConfigInvalidValue: "Valor no válido %q para %s. Permitidos: %s.",
	ConfigSetDone:      "%s = %s",
	ConfigUnsetDone:    "%s restablecido a su valor predeterminado: %s",

	NoticeReset:         "Aviso: no se pudo leer %s. Se movió a %s y cid empezó con uno vacío.",
	NoticeRestored:      "Aviso: no se pudo leer %s. Se movió a %s y se restauró desde la última copia válida.",
	NoticeActiveCleared: "Aviso: el marcador de perfil activo apuntaba a un perfil inexistente (%s) y se borró.",
	NoticeUnknownKey:    "Aviso: %s tiene un ajuste desconocido %q, se ignora.",
	NoticeInvalidValue:  "Aviso: %s tiene un valor no válido para %q, se usa el predeterminado.",
}

func init() {
	for k, v := range configEN {
		catalogEN[k] = v
	}
	for k, v := range configES {
		catalogES[k] = v
	}
}
