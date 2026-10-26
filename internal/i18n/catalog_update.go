package i18n

// Message keys for `cid update` and the release notice in `cid version`.
const (
	UpdateAvailable  Key = "update.available"
	UpdateUpToDate   Key = "update.up_to_date"
	UpdateDone       Key = "update.done"
	UpdateFailed     Key = "update.failed"
	UpdateDevBuild   Key = "update.dev_build"
	UpdatePluginDone Key = "update.plugin_done"
	UpdatePluginWarn Key = "update.plugin_warn"
)

var updateEN = map[Key]string{
	UpdateAvailable:  "cid %s is available. Run: cid update",
	UpdateUpToDate:   "cid %s is already the latest version.",
	UpdateDone:       "Updated cid from %s to %s.",
	UpdateFailed:     "Could not update cid: %s.",
	UpdateDevBuild:   "Cannot update a development build of cid.",
	UpdatePluginDone: "Plugin account-switch updated. Restart Claude Code or run /reload-plugins to load it.",
	UpdatePluginWarn: "Warning: could not update the account-switch plugin: %s.",
}

var updateES = map[Key]string{
	UpdateAvailable:  "Está disponible cid %s. Ejecuta: cid update",
	UpdateUpToDate:   "cid %s ya es la versión más reciente.",
	UpdateDone:       "cid actualizado de %s a %s.",
	UpdateFailed:     "No se pudo actualizar cid: %s.",
	UpdateDevBuild:   "No se puede actualizar una compilación de desarrollo de cid.",
	UpdatePluginDone: "Plugin account-switch actualizado. Reinicia Claude Code o ejecuta /reload-plugins para cargarlo.",
	UpdatePluginWarn: "Aviso: no se pudo actualizar el plugin account-switch: %s.",
}

func init() {
	for k, v := range updateEN {
		catalogEN[k] = v
	}
	for k, v := range updateES {
		catalogES[k] = v
	}
}
