package i18n

// Message keys for `cid statusline install` and `cid statusline uninstall`.
const (
	StatuslineInstalled   Key = "statusline.installed"
	StatuslineAlready     Key = "statusline.already"
	StatuslineRemoved     Key = "statusline.removed"
	StatuslineNotOurs     Key = "statusline.not_ours"
	StatuslineNoBackup    Key = "statusline.no_backup"
	StatuslineBadSettings Key = "statusline.bad_settings"
	StatuslineUnsupported Key = "statusline.unsupported"
)

var statuslineEN = map[Key]string{
	StatuslineInstalled:   "Status line wrapper installed in %s.",
	StatuslineAlready:     "The status line is already wrapped by cid; nothing changed.",
	StatuslineRemoved:     "Status line restored in %s.",
	StatuslineNotOurs:     "The status line is not the cid wrapper; nothing changed.",
	StatuslineNoBackup:    "The saved copy of the previous status line is missing; nothing changed.",
	StatuslineBadSettings: "%s is not a valid JSON object; nothing changed.",
	StatuslineUnsupported: "The current status line cannot be wrapped safely (unexpected shape or shell syntax); nothing changed.",
}

var statuslineES = map[Key]string{
	StatuslineInstalled:   "Envoltorio de la línea de estado instalado en %s.",
	StatuslineAlready:     "La línea de estado ya está envuelta por cid; no se cambió nada.",
	StatuslineRemoved:     "Línea de estado restaurada en %s.",
	StatuslineNotOurs:     "La línea de estado no es el envoltorio de cid; no se cambió nada.",
	StatuslineNoBackup:    "Falta la copia guardada de la línea de estado anterior; no se cambió nada.",
	StatuslineBadSettings: "%s no es un objeto JSON válido; no se cambió nada.",
	StatuslineUnsupported: "La línea de estado actual no se puede envolver de forma segura (formato inesperado o sintaxis de shell); no se cambió nada.",
}

func init() {
	for k, v := range statuslineEN {
		catalogEN[k] = v
	}
	for k, v := range statuslineES {
		catalogES[k] = v
	}
}
