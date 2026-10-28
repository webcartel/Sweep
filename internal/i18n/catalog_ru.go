package i18n

// catalogRU is the Russian text of every key of catalogEN. A key that is
// missing here falls back to English at lookup time, and
// TestCatalogsHoldTheSameKeys fails until it is added.
var catalogRU = map[string]string{
	"app.title": "Sweep - безопасно освобождаем место",

	"tui.button.back": "Назад",

	"tui.hint.back": "Esc назад",

	"tui.lang.title":      "Язык",
	"tui.lang.hint":       "Выбор сохранится и будет использован при следующем запуске.",
	"tui.lang.current":    "Сейчас: %s",
	"tui.lang.saved":      "Сохранено.",
	"tui.lang.savefailed": "Не удалось сохранить выбор",
	"tui.lang.nosettings": "Нет файла настроек для записи.",

	"tui.rules.title":          "Правила и настройки",
	"tui.rules.intro":          "Правила определяют, что ищет sweep. Встроенные правила менять нельзя.",
	"tui.rules.counts":         "Правила:    %d встроенных, %d ваших",
	"tui.rules.profiles":       "Профили:    %d доступно",
	"tui.rules.config":         "Конфиг:     %s (%s)",
	"tui.rules.confignotfound": "не найден - действуют значения по умолчанию",
	"tui.rules.configfound":    "найден",
	"tui.rules.yourrules":      "Ваши правила: %s",
	"tui.rules.holds":          "Что есть на каждом экране:",
	"tui.rules.holds.browse":   "Обзор правил   - все правила, их пути и происхождение",
	"tui.rules.holds.config":   "Конфиг         - профили, переключатели и исключённые пути",
	"tui.rules.holds.lang":     "Язык           - язык, на котором говорят экраны",
	"tui.rules.holds.mine":     "Мои правила    - ваши файлы: создание, правка, удаление",
	"tui.rules.undo":           "Правку можно отменить: каждое сохранение оставляет рядом файл .bak.",
	"tui.rules.hint.browse":    "B поиск",
	"tui.rules.hint.config":    "C конфиг",
	"tui.rules.hint.lang":      "L язык",
	"tui.rules.hint.mine":      "U мои правила",
	"tui.rules.browse":         "Обзор правил",
	"tui.rules.viewconfig":     "Конфиг",
	"tui.rules.language":       "Язык",
	"tui.rules.mine":           "Мои правила",
}
