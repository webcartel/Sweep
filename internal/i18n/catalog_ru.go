package i18n

// catalogRU is the Russian text of every key of catalogEN. A key that is
// missing here falls back to English at lookup time, and
// TestCatalogsHoldTheSameKeys fails until it is added.
var catalogRU = map[string]string{
	"app.title": "Sweep - безопасно освобождаем место",
}
