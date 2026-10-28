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

	"registry.note.read":        "не удалось прочитать %s: %v",
	"registry.note.var":         "неизвестная переменная %%%s%% в %q",
	"registry.note.relative":    "относительную команду %q нельзя прочитать как место на диске",
	"registry.note.path":        "не удалось прочитать %q: %v",
	"registry.note.drive":       "не удалось прочитать диск %q: %v",
	"registry.note.removable":   "%q находится не на локальном диске; пропущено",
	"registry.note.guard":       "вне защищённых веток",
	"registry.note.subkeys":     "содержит подключи",
	"registry.note.backup":      "не удалось создать резервную копию: %s",
	"registry.err.backup":       "не задан каталог для резервных копий",
	"registry.reason.uninstall": "команда удаления не найдена: %s",
	"registry.reason.program":   "программа не найдена: %s",

	"scan.skip.var":         "переменная %s не задана",
	"scan.skip.mask":        "ни одного пути из маски нет на диске",
	"scan.skip.notfound":    "путь не существует",
	"scan.skip.contents":    "цель — файл, а режим contents чистит каталоги",
	"scan.skip.disappeared": "путь исчез",
	"scan.skip.build":       "правило требует сборку %d, а здесь Windows со сборкой %d",
	"scan.warn.walk":        "правило не удалось обойти: %v",

	"guard.err.empty":          "путь пуст",
	"guard.err.var":            "путь %s называет %%%s%%, которую эта машина не задаёт",
	"guard.err.nopath":         "путь %s не называет никакого места",
	"guard.err.loop":           "ссылки %s ведут обратно сами в себя",
	"guard.err.depth":          "ссылки %s глубже %d",
	"guard.err.disk":           "путь %s находится не на локальном фиксированном диске",
	"guard.err.exclude.var":    "exclude_paths %s называет %%%s%%, которую эта машина не задаёт",
	"guard.err.exclude.nopath": "exclude_paths %s не называет никакого места",
	"guard.err.exclude.mask":   "exclude_paths %s — маска, а список принимает пути",
}
