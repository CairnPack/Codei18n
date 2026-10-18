package langdetect

import (
	"sync"
	"testing"

	"github.com/CairnPack/codei18n/core/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// detectCase 描述一条注释样本及期望结果；source 为空表示不检查来源。
type detectCase struct {
	name   string
	text   string
	lang   string
	source string
}

const (
	srcDet = index.LangSourceDetected
	srcHeu = index.LangSourceHeuristic
	srcFb  = index.LangSourceFallback
)

// defaultCases 是默认候选集下的真实风格注释样本，覆盖 9 种语言、短注释、代码主导的注释、
// 各类文档注释（Javadoc/KDoc/JSDoc/C# XML）以及中英混排。
var defaultCases = []detectCase{
	// —— 短注释：走文字系统启发式 ——
	{"短英文无法判断", "// TODO fix", "unknown", srcFb},
	{"短英文两词", "// fix typo", "unknown", srcFb},
	{"简体短注释", "// 修复", "zh-Hans", srcHeu},
	{"繁体短注释", "// 修復", "zh-Hant", srcHeu},
	{"日文片假名短注释", "// テスト", "ja", srcHeu},
	{"韩文短注释", "// 테스트", "ko", srcHeu},
	{"俄文短注释", "// исправить ошибку", "ru", srcHeu},
	{"简繁共用字短注释取简体", "// 返回值", "zh-Hans", srcHeu},
	{"只剩标识符", "// see fooBar() in pkg/util.go", "unknown", srcFb},
	{"只剩代码与链接", "// `x := y` https://example.com/a?b=c", "unknown", srcFb},
	{"空注释", "//", "unknown", srcFb},
	{"中文夹标识符的短注释", "// 调用fooBar()", "zh-Hans", srcHeu},
	{"中文夹标识符的长注释", "// 调用fooBar()来初始化缓存", "zh-Hans", srcDet},
	{"只含 @param 标签的简体", "/** @param id 编号 */", "zh-Hans", srcHeu},
	{"短德文无法判断", "// Wert prüfen", "unknown", srcFb},
	{"只含汉字的日文按中文处理（已知局限，且新字体常与繁体字形重合）", "// 東京都庁舎", "zh-Hant", srcDet},
	{"TODO 加简体", "// TODO: 修复", "zh-Hans", srcHeu},

	// —— 英文 ——
	{"英文行注释", "// Returns the number of bytes written to the underlying buffer.", "en", srcDet},
	{"英文含标识符", "// parseConfig reads the YAML file at cfgPath and validates every field.", "en", srcDet},
	{"英文块注释", "/* This function is not thread-safe; callers must hold the lock. */", "en", srcDet},
	{"英文 Javadoc", "/**\n * Creates a new connection pool with the given size.\n *\n * @param size the maximum number of connections\n * @return a configured {@link ConnectionPool}\n * @throws IllegalArgumentException if size is negative\n */", "en", srcDet},
	{"英文 KDoc", "/**\n * Loads the user profile from the remote server and caches it locally.\n * See [UserRepository.load] for details.\n */", "en", srcDet},
	{"英文 JSDoc", "/**\n * Formats a date according to the current locale.\n * @param {Date} date - the date to format\n * @returns {string} the formatted string\n */", "en", srcDet},
	{"英文 C# XML 文档", "/// <summary>\n/// Gets or sets the timeout used when waiting for a response.\n/// </summary>\n/// <param name=\"value\">The timeout in <c>TimeSpan</c> units.</param>", "en", srcDet},
	{"英文含围栏代码", "/**\n * Example usage of the client:\n * ```\n * client := New(cfg)\n * ```\n * The client must be closed after use.\n */", "en", srcDet},

	// —— 简体中文 ——
	{"简体行注释", "// 初始化连接池，然后处理请求", "zh-Hans", srcDet},
	{"简体 Javadoc", "/**\n * 根据用户 ID 查询用户信息。\n * @param userId 用户 ID\n * @return 用户对象，不存在时返回 null\n */", "zh-Hans", srcDet},
	{"简体混英文术语", "// 这里使用 LRU cache 来缓存最近访问的结果，避免重复计算", "zh-Hans", srcDet},
	{"简体含代码", "// 当 `ctx.Done()` 被关闭时，立即停止所有后台任务并释放资源", "zh-Hans", srcDet},

	// —— 繁体中文 ——
	{"繁体行注释", "// 初始化連線池，然後處理請求", "zh-Hant", srcDet},
	{"繁体文档注释", "/**\n * 檢查設定檔是否存在，若不存在則建立預設設定。\n */", "zh-Hant", srcDet},
	{"繁体混英文", "// 這個函式會回傳 HTTP 狀態碼與錯誤訊息", "zh-Hant", srcDet},

	// —— 日文 ——
	{"日文行注释", "// 設定ファイルを読み込んで、デフォルト値を適用する", "ja", srcDet},
	{"日文文档注释", "/**\n * ユーザー情報を取得します。見つからない場合は null を返します。\n */", "ja", srcDet},
	{"日文混英文", "// ここで cache を更新しないと、古いデータが表示されてしまう", "ja", srcDet},

	// —— 韩文 ——
	{"韩文行注释", "// 설정 파일을 읽고 기본값을 적용합니다", "ko", srcDet},
	{"韩文文档注释", "/**\n * 사용자 정보를 조회합니다. 존재하지 않으면 null을 반환합니다.\n */", "ko", srcDet},

	// —— 德文 ——
	{"德文行注释", "// Gibt die Anzahl der geschriebenen Bytes zurück.", "de", srcDet},
	{"德文 Javadoc", "/**\n * Erstellt eine neue Verbindung zur Datenbank und prüft die Zugangsdaten.\n * @param url die Adresse der Datenbank\n * @throws SQLException wenn die Verbindung fehlschlägt\n */", "de", srcDet},
	{"德文 C# XML 文档", "/// <summary>\n/// Liest die Konfiguration aus der Datei und überschreibt die Standardwerte.\n/// </summary>", "de", srcDet},

	// —— 法文 ——
	{"法文行注释", "// Retourne le nombre d'octets écrits dans le tampon.", "fr", srcDet},
	{"法文 KDoc", "/**\n * Charge le profil de l'utilisateur depuis le serveur distant.\n * Voir [UserRepository] pour plus de détails.\n */", "fr", srcDet},
	{"法文 JSDoc", "/**\n * Vérifie que la valeur est comprise entre les deux bornes.\n * @param {number} min - la borne inférieure\n */", "fr", srcDet},

	// —— 西班牙文 ——
	{"西文行注释", "// Devuelve el número de elementos que hay en la lista.", "es", srcDet},
	{"西文 Javadoc", "/**\n * Calcula el precio total del pedido, incluidos los impuestos.\n * @param pedido el pedido que se va a procesar\n */", "es", srcDet},
	{"西文 C# XML 文档", "/// <summary>\n/// Obtiene la configuración actual de la aplicación.\n/// </summary>", "es", srcDet},

	// —— 俄文 ——
	{"俄文行注释", "// Возвращает количество записанных байтов в буфер.", "ru", srcDet},
	{"俄文文档注释", "/**\n * Создаёт новое подключение к базе данных.\n * @param url адрес сервера\n */", "ru", srcDet},
}

func TestDetectDefaultCandidates(t *testing.T) {
	d, err := New(nil)
	require.NoError(t, err)
	for _, tc := range defaultCases {
		t.Run(tc.name, func(t *testing.T) {
			got := d.Detect(tc.text)
			assert.Equal(t, tc.lang, got.Lang, "注释：%q，清洗后：%q", tc.text, cleanComment(index.Normalize(tc.text)))
			if tc.source != "" {
				assert.Equal(t, tc.source, got.Source)
			}
		})
	}
}

// TestCandidateRestriction 验证结果只会落在候选集内（或 unknown）。
func TestCandidateRestriction(t *testing.T) {
	cases := []struct {
		name  string
		cands []string
		text  string
		want  string
	}{
		{"日文在 en/zh-Hans 下不返回 ja", []string{"en", "zh-Hans"}, "// 設定ファイルを読み込んで、デフォルト値を適用する", "unknown"},
		{"日文短注释在 en/zh-Hans 下不返回 ja", []string{"en", "zh-Hans"}, "// テスト", "unknown"},
		{"韩文在 en/zh-Hans 下不返回 ko", []string{"en", "zh-Hans"}, "// 설정 파일을 읽고 기본값을 적용합니다", "unknown"},
		{"俄文短注释不在候选中", []string{"en", "de"}, "// исправить ошибку", "unknown"},
		{"英文仍可识别", []string{"en", "zh-Hans"}, "// Returns the number of bytes written to the underlying buffer.", "en"},
		{"繁体文本在只有简体候选时归为简体", []string{"en", "zh-Hans"}, "// 初始化連線池，然後處理請求", "zh-Hans"},
		{"简体文本在只有繁体候选时归为繁体", []string{"en", "zh-Hant"}, "// 修复", "zh-Hant"},
		{"共用字在只有繁体候选时取繁体", []string{"zh-Hant", "en"}, "// 返回值", "zh-Hant"},
		{"候选限定语义：拉丁文字只有 en 一个候选时德文也归为 en", []string{"en", "zh-Hans"}, "// Gibt die Anzahl der geschriebenen Bytes zurück.", "en"},
		{"单一候选：英文", []string{"en"}, "// Returns the number of bytes written to the underlying buffer.", "en"},
		{"单一候选 en：同为拉丁文字的德文归为 en", []string{"en"}, "// Erstellt eine neue Verbindung zur Datenbank und prüft die Zugangsdaten.", "en"},
		{"单一候选 en：俄文被拒识", []string{"en"}, "// Возвращает количество записанных байтов в буфер.", "unknown"},
		{"单一候选 ru：英文被拒识", []string{"ru"}, "// Returns the number of bytes written to the underlying buffer.", "unknown"},
		{"单一候选 zh-Hans：英文被拒识", []string{"zh-Hans"}, "// Returns the number of bytes written to the underlying buffer.", "unknown"},
		{"单一候选：中文", []string{"zh-Hans"}, "// 初始化连接池，然后处理请求", "zh-Hans"},
		{"只含中文两种变体", []string{"zh-Hans", "zh-Hant"}, "// 初始化連線池，然後處理請求", "zh-Hant"},
		{"候选写法会被规范化", []string{"EN", "zh_TW"}, "// 初始化連線池，然後處理請求", "zh-Hant"},
		{"zh-CN 规范化为简体", []string{"en-US", "zh-CN"}, "// 初始化连接池，然后处理请求", "zh-Hans"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := New(tc.cands)
			require.NoError(t, err)
			assert.Equal(t, tc.want, d.Detect(tc.text).Lang)
		})
	}
}

// TestNewErrors 验证不支持的候选语言返回列出支持语言码的错误。
func TestNewErrors(t *testing.T) {
	for _, cands := range [][]string{{"en", "it"}, {"pt-BR"}, {"en", ""}, {"unknown"}} {
		_, err := New(cands)
		require.Error(t, err, "%v", cands)
		for _, code := range Supported() {
			assert.Contains(t, err.Error(), code)
		}
	}
}

// TestSupported 验证支持列表为规范化语言码，且返回副本。
func TestSupported(t *testing.T) {
	got := Supported()
	assert.Equal(t, []string{"en", "zh-Hans", "zh-Hant", "ja", "ko", "de", "fr", "es", "ru"}, got)
	got[0] = "xx"
	assert.Equal(t, "en", Supported()[0])
}

// TestCleanComment 验证清洗规则：代码、标签、链接、路径、标识符、数字与标点都被去掉。
func TestCleanComment(t *testing.T) {
	cases := []struct{ in, want string }{
		{"// see fooBar() in pkg/util.go", "see in"},
		{"// Call `client.Close()` when done", "Call when done"},
		{"/** Uses {@link java.util.Map} and {@code null}. */", "Uses and"},
		{"/** @param userName the name @throws IOException on error */", "the name on error"},
		{"/** @param {string} id - the id */", "the id"},
		{"/// <summary>Gets the <c>Foo</c> value.</summary> <see cref=\"Bar\"/>", "Gets the value"},
		{"// See [UserRepo.load] or [docs](https://x.io/a) for details", "See or docs for details"},
		{"// visit https://example.com/path?q=1 or www.example.org now", "visit or now"},
		{"// MAX_SIZE is 1024, use HTTPServer or parseJSON or v1.2 or os.Path", "is use or or or"},
		{`// TODO: FIXME 42 times, and/or C:\temp`, "times"},
		{"// 调用fooBar()来初始化，见 util.go。", "调用 来初始化 见"},
		{"// l'objet n'est pas well-known", "l'objet n'est pas well-known"},
		{`// \brief Returns the value \param x input`, "Returns the value input"},
		{`""" :param name: the name :returns: value """`, "the name value"},
		{"/* ```\nfoo := bar()\n``` Done */", "Done"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, cleanComment(index.Normalize(tc.in)), tc.in)
	}
}

// TestLegacyUnchanged 确认旧版检测器行为未变。
func TestLegacyUnchanged(t *testing.T) {
	assert.Equal(t, Result{Lang: "zh-Hans", Source: srcHeu}, Legacy{}.Detect("// 修復"))
	assert.Equal(t, Result{Lang: "unknown", Source: srcFb}, Legacy{}.Detect("// fix"))
}

// TestConcurrent 在同一个 Detector 上并发识别（配合 -race 运行）。
func TestConcurrent(t *testing.T) {
	d, err := New(nil)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				for _, tc := range defaultCases {
					assert.Equal(t, tc.lang, d.Detect(tc.text).Lang, tc.name)
				}
			}
		}()
	}
	wg.Wait()
}
