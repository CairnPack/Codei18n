package langdetect

import "testing"

// evalCorpus 是 3~10 词的英/德/法/西短注释语料，用于回归统计模型部分的准确率
// （minRelativeDistance 即据此调参，见 detect.go）。
var evalCorpus = map[string][]string{
	"en": {"Close the file when done", "Handle edge cases first", "This method is not implemented yet", "Ignore errors from the cleanup step",
		"Wait for all workers to finish", "Check whether the value is null", "Read the docs before changing this", "Cache the result for later use",
		"Retry the request on failure", "Only used in unit tests", "Remove this after the migration", "Returns the name of the user",
		"The caller owns the returned slice", "Do not modify the input array", "Skip hidden files and directories", "Make sure the buffer is flushed",
		"Keep this in sync with the server", "Called once per request", "Lazily initialized on first access", "Safe for concurrent use",
		"Number of retries before giving up", "Convert the timestamp to local time", "Fallback when the config is missing", "Used to break ties between equal scores"},
	"de": {"Datei nach Gebrauch schließen", "Sonderfälle zuerst behandeln", "Diese Methode ist noch nicht implementiert", "Fehler beim Aufräumen ignorieren",
		"Warten bis alle Worker fertig sind", "Prüfen ob der Wert null ist", "Vor dem Ändern die Doku lesen", "Ergebnis für später zwischenspeichern",
		"Anfrage bei Fehler wiederholen", "Nur in Unit-Tests verwendet", "Nach der Migration entfernen", "Liefert den Namen des Benutzers",
		"Der Aufrufer besitzt die zurückgegebene Liste", "Das Eingabearray nicht verändern", "Versteckte Dateien und Ordner überspringen", "Sicherstellen dass der Puffer geleert wird",
		"Muss mit dem Server synchron bleiben", "Wird einmal pro Anfrage aufgerufen", "Wird beim ersten Zugriff initialisiert", "Kann gleichzeitig von mehreren Threads benutzt werden",
		"Anzahl der Versuche bevor abgebrochen wird", "Zeitstempel in lokale Zeit umwandeln", "Standardwert falls die Konfiguration fehlt", "Hier keine Ausnahme werfen"},
	"fr": {"Fermer le fichier après usage", "Traiter les cas limites en premier", "Cette méthode n'est pas encore implémentée", "Ignorer les erreurs du nettoyage",
		"Attendre que tous les workers aient terminé", "Vérifier si la valeur est nulle", "Lire la doc avant de modifier ceci", "Mettre le résultat en cache pour plus tard",
		"Réessayer la requête en cas d'échec", "Utilisé uniquement dans les tests", "Supprimer après la migration", "Renvoie le nom de l'utilisateur",
		"L'appelant possède la liste renvoyée", "Ne pas modifier le tableau d'entrée", "Ignorer les fichiers et dossiers cachés", "S'assurer que le tampon est vidé",
		"Doit rester synchronisé avec le serveur", "Appelé une fois par requête", "Initialisé au premier accès", "Peut être utilisé par plusieurs threads",
		"Nombre de tentatives avant d'abandonner", "Convertir l'horodatage en heure locale", "Valeur par défaut si la configuration manque", "Ne jamais lever d'exception ici"},
	"es": {"Cerrar el archivo al terminar", "Tratar primero los casos límite", "Este método todavía no está implementado", "Ignorar los errores de la limpieza",
		"Esperar a que terminen todos los workers", "Comprobar si el valor es nulo", "Leer la documentación antes de cambiar esto", "Guardar el resultado en caché",
		"Reintentar la petición si falla", "Solo se usa en las pruebas", "Eliminar después de la migración", "Devuelve el nombre del usuario",
		"El llamador es dueño de la lista devuelta", "No modificar el arreglo de entrada", "Omitir archivos y carpetas ocultos", "Asegurarse de vaciar el búfer",
		"Debe mantenerse sincronizado con el servidor", "Se llama una vez por petición", "Se inicializa en el primer acceso", "Puede usarse desde varios hilos a la vez",
		"Número de intentos antes de rendirse", "Convertir la marca de tiempo a hora local", "Valor por defecto si falta la configuración", "Nunca lanzar una excepción aquí"},
}

// TestLatinAccuracy 要求语料零误判，且因置信度不足落入 unknown 的不超过 3 条。
func TestLatinAccuracy(t *testing.T) {
	d, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	total, unknownCount := 0, 0
	for want, ss := range evalCorpus {
		for _, s := range ss {
			total++
			switch got := d.Detect("// " + s).Lang; got {
			case want:
			case "unknown":
				unknownCount++
				t.Logf("未识别（%s）：%q", want, s)
			default:
				t.Errorf("误判 %s → %s：%q", want, got, s)
			}
		}
	}
	if unknownCount > 3 {
		t.Errorf("%d 条中有 %d 条落入 unknown，超过上限 3", total, unknownCount)
	}
}

// BenchmarkDetect 在默认候选集上混合测量各类注释（短注释、文档注释、各语言）的单次识别开销。
func BenchmarkDetect(b *testing.B) {
	d, err := New(nil)
	if err != nil {
		b.Fatal(err)
	}
	var all []string
	for _, ss := range evalCorpus {
		for _, s := range ss {
			all = append(all, "// "+s)
		}
	}
	for _, tc := range defaultCases {
		all = append(all, tc.text)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Detect(all[i%len(all)])
	}
}

// BenchmarkNew 测量创建检测器（含预加载模型）的开销。
func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := New(nil); err != nil {
			b.Fatal(err)
		}
	}
}
