package i18n

// Message keys for the usage lines of `cid list`.
const (
	UsageLabelFiveHour Key = "usage.label_five_hour"
	UsageLabelWeek     Key = "usage.label_week"
	UsageResets        Key = "usage.resets"
	UsageAgoMinutes    Key = "usage.ago_minutes"
	UsageAgoHours      Key = "usage.ago_hours"
	UsageAgoDays       Key = "usage.ago_days"
	UsageHint          Key = "usage.hint"
	UsageActive        Key = "usage.active"
)

var listEN = map[Key]string{
	UsageLabelFiveHour: "5h",
	UsageLabelWeek:     "week",
	UsageResets:        "resets %s",
	UsageAgoMinutes:    "%dm ago",
	UsageAgoHours:      "%dh ago",
	UsageAgoDays:       "%dd ago",
	UsageActive:        "active",
	UsageHint:          "No usage recorded yet. Run `cid statusline install` to see plan usage here.",
}

var listES = map[Key]string{
	UsageLabelFiveHour: "5h",
	UsageLabelWeek:     "semana",
	UsageResets:        "se reinicia en %s",
	UsageAgoMinutes:    "hace %d min",
	UsageAgoHours:      "hace %d h",
	UsageAgoDays:       "hace %d d",
	UsageActive:        "activa",
	UsageHint:          "Aún no hay uso registrado. Ejecuta `cid statusline install` para ver aquí el uso del plan.",
}

func init() {
	for k, v := range listEN {
		catalogEN[k] = v
	}
	for k, v := range listES {
		catalogES[k] = v
	}
}
