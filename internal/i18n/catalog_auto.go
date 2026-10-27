package i18n

// Message keys for `cid auto`.
const (
	AutoStatusOn          Key = "auto.status_on"
	AutoStatusOff         Key = "auto.status_off"
	AutoNoAccounts        Key = "auto.no_accounts"
	AutoAccountLine       Key = "auto.account_line"
	AutoRolePrincipal     Key = "auto.role_principal"
	AutoRoleReserve       Key = "auto.role_reserve"
	AutoPendingLine       Key = "auto.pending_line"
	AutoReasonReturn      Key = "auto.reason_return"
	AutoLastSwitch        Key = "auto.last_switch"
	AutoOn                Key = "auto.on"
	AutoOff               Key = "auto.off"
	AutoAdded             Key = "auto.added"
	AutoRemoved           Key = "auto.removed"
	AutoNotEnrolled       Key = "auto.not_enrolled"
	AutoPrincipalSet      Key = "auto.principal_set"
	AutoReserveSet        Key = "auto.reserve_set"
	AutoDefaultsSet       Key = "auto.defaults_set"
	AutoInvalidPercentage Key = "auto.invalid_percentage"
)

var autoEN = map[Key]string{
	AutoStatusOn:          "Auto-switch: on (defaults: 5h %d%%, week %d%%)",
	AutoStatusOff:         "Auto-switch: off (defaults: 5h %d%%, week %d%%)",
	AutoNoAccounts:        "No accounts enrolled. Add one with `cid auto add <n|name>`.",
	AutoAccountLine:       "%d  %s  5h %d%%  week %d%%%s",
	AutoRolePrincipal:     "principal",
	AutoRoleReserve:       "reserve",
	AutoPendingLine:       "Pending switch: %s → %s (%s)",
	AutoReasonReturn:      "back to the principal",
	AutoLastSwitch:        "Last automatic switch: %s",
	AutoOn:                "Auto-switch is on.",
	AutoOff:               "Auto-switch is off.",
	AutoAdded:             "Account %q enrolled (5h %d%%, week %d%%).",
	AutoRemoved:           "Account %q removed from auto-switch.",
	AutoNotEnrolled:       "Account %q is not enrolled. Add it with `cid auto add %s`.",
	AutoPrincipalSet:      "Account %q is now the principal.",
	AutoReserveSet:        "Account %q is now a reserve.",
	AutoDefaultsSet:       "Defaults: 5h %d%%, week %d%%.",
	AutoInvalidPercentage: "Invalid percentage %q: use a whole number from 1 to 100.",
}

var autoES = map[Key]string{
	AutoStatusOn:          "Cambio automático: activado (por defecto: 5h %d%%, semana %d%%)",
	AutoStatusOff:         "Cambio automático: desactivado (por defecto: 5h %d%%, semana %d%%)",
	AutoNoAccounts:        "No hay cuentas inscritas. Agrega una con `cid auto add <n|nombre>`.",
	AutoAccountLine:       "%d  %s  5h %d%%  semana %d%%%s",
	AutoRolePrincipal:     "principal",
	AutoRoleReserve:       "reserva",
	AutoPendingLine:       "Cambio pendiente: %s → %s (%s)",
	AutoReasonReturn:      "vuelta a la principal",
	AutoLastSwitch:        "Último cambio automático: %s",
	AutoOn:                "El cambio automático está activado.",
	AutoOff:               "El cambio automático está desactivado.",
	AutoAdded:             "Cuenta %q inscrita (5h %d%%, semana %d%%).",
	AutoRemoved:           "Cuenta %q quitada del cambio automático.",
	AutoNotEnrolled:       "La cuenta %q no está inscrita. Agrégala con `cid auto add %s`.",
	AutoPrincipalSet:      "La cuenta %q es ahora la principal.",
	AutoReserveSet:        "La cuenta %q es ahora una reserva.",
	AutoDefaultsSet:       "Valores por defecto: 5h %d%%, semana %d%%.",
	AutoInvalidPercentage: "Porcentaje no válido %q: usa un número entero de 1 a 100.",
}

func init() {
	for k, v := range autoEN {
		catalogEN[k] = v
	}
	for k, v := range autoES {
		catalogES[k] = v
	}
}
