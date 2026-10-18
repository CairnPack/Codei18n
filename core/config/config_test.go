package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustFile(t *testing.T, s string) *File {
	t.Helper()
	var f File
	if err := json.Unmarshal([]byte(s), &f); err != nil {
		t.Fatal(err)
	}
	return &f
}

func TestResolveDefaults(t *testing.T) {
	e := Resolve(nil, nil, Overrides{})
	if e.SourceLanguage.Value != "en" || e.SourceLanguage.Source != LayerDefault {
		t.Fatalf("%+v", e.SourceLanguage)
	}
	if e.Provider.Name.Value != "openai" || e.Provider.Model.Value != DefaultOpenAIModel {
		t.Fatalf("%+v", e.Provider)
	}
	if e.Provider.BatchSize.Value != 10 || e.Provider.Concurrency.Value != 5 {
		t.Fatalf("%+v", e.Provider)
	}
	if e.ProfileName() != "default" {
		t.Fatal(e.ProfileName())
	}
}

func TestResolveLayerPrecedenceAndSources(t *testing.T) {
	global := mustFile(t, `{"sourceLanguage":"fr","localLanguage":"de","batchSize":20}`)
	project := mustFile(t, `{"localLanguage":"ja","projectId":"p1"}`)
	ov := Overrides{SourceLanguage: Ptr("es")}
	e := Resolve(global, project, ov)
	check := func(name string, v Value[string], want string, src Layer) {
		t.Helper()
		if v.Value != want || v.Source != src {
			t.Errorf("%s = %+v, want %s from %s", name, v, want, src)
		}
	}
	check("source", e.SourceLanguage, "es", LayerOverride)
	check("local", e.LocalLanguage, "ja", LayerProject)
	if e.Provider.BatchSize.Value != 20 || e.Provider.BatchSize.Source != LayerGlobal {
		t.Errorf("batch %+v", e.Provider.BatchSize)
	}
	if e.ProjectID != "p1" {
		t.Error("projectId")
	}
}

func TestResolveProfiles(t *testing.T) {
	global := mustFile(t, `{
		"translationProvider":"openai","translationConfig":{"model":"gpt-4"},
		"profile":"ds",
		"profiles":{
			"ds":{"provider":"openai","baseUrl":"https://api.deepseek.com","model":"deepseek-chat","concurrency":8},
			"local":{"provider":"ollama","model":"qwen"}
		}}`)
	e := Resolve(global, nil, Overrides{})
	if e.Profile.Value != "ds" || e.Provider.Model.Value != "deepseek-chat" || e.Provider.Concurrency.Value != 8 {
		t.Fatalf("%+v %+v", e.Profile, e.Provider)
	}
	if e.Provider.Model.Source != LayerGlobal {
		t.Fatal("profile 字段来源应为 global")
	}

	// 项目选用 local，并覆盖单个字段；项目里旧 init 的快照字段被忽略。
	project := mustFile(t, `{"profile":"local","translationProvider":"openai","translationConfig":{"model":"gpt-4"},
		"profileOverrides":{"model":"qwen2.5:7b"}}`)
	e = Resolve(global, project, Overrides{})
	if e.Provider.Name.Value != "ollama" || e.Provider.Name.Source != LayerGlobal {
		t.Fatalf("provider %+v", e.Provider.Name)
	}
	if e.Provider.Model.Value != "qwen2.5:7b" || e.Provider.Model.Source != LayerProject {
		t.Fatalf("model %+v", e.Provider.Model)
	}
	if e.Provider.Endpoint.Value != DefaultOllamaEndpoint || e.Provider.Endpoint.Source != LayerDefault {
		t.Fatalf("endpoint %+v", e.Provider.Endpoint)
	}

	// 调用方覆盖优先级最高。
	e = Resolve(global, project, Overrides{Provider: ProviderFields{Model: Ptr("x")}})
	if e.Provider.Model.Value != "x" || e.Provider.Model.Source != LayerOverride {
		t.Fatalf("override %+v", e.Provider.Model)
	}

	e = Resolve(global, mustFile(t, `{"profile":"nope"}`), Overrides{})
	if !e.ProfileMissing {
		t.Fatal("应标记 profile 缺失")
	}
}

func TestLegacyProjectFieldsStillApplyWithoutProfile(t *testing.T) {
	project := mustFile(t, `{"translationProvider":"ollama","translationConfig":{"Endpoint":"http://h:1","MODEL":"m"}}`)
	e := Resolve(nil, project, Overrides{})
	if e.Provider.Name.Value != "ollama" || e.Provider.Endpoint.Value != "http://h:1" || e.Provider.Model.Value != "m" {
		t.Fatalf("%+v", e.Provider)
	}
	legacy := e.Legacy()
	if legacy.TranslationProvider != "ollama" || legacy.TranslationConfig["endpoint"] != "http://h:1" {
		t.Fatalf("%+v", legacy)
	}
}

func TestFilePreservesUnknownFieldsAndOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte("\xef\xbb\xbf"+`{"sourceLanguage":"en","futureField":{"a":1},
		"profiles":{"p":{"model":"m","futureProfileField":true}}}`), 0o644)
	if err := UpdateFile(path, func(f *File) error {
		f.LocalLanguage = Ptr("ja")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	for _, want := range []string{`"futureField": {`, `"futureProfileField": true`, `"localLanguage": "ja"`} {
		if !strings.Contains(s, want) {
			t.Errorf("缺少 %s:\n%s", want, s)
		}
	}
	if strings.Index(s, "sourceLanguage") > strings.Index(s, "futureField") {
		t.Error("已知字段应排在未知字段之前")
	}
}

func TestReadFileMissingAndEmpty(t *testing.T) {
	dir := t.TempDir()
	f, err := ReadFile(filepath.Join(dir, "none.json"))
	if err != nil || f == nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "empty.json"), []byte("  "), 0o644)
	if _, err := ReadFile(filepath.Join(dir, "empty.json")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o644)
	if _, err := ReadFile(filepath.Join(dir, "bad.json")); err == nil {
		t.Fatal("应报错")
	}
}

func TestSanitizeDropsSecrets(t *testing.T) {
	c := &Config{TranslationConfig: map[string]string{"api_key": "x", "APIKEY": "y", "token": "z", "model": "m"}}
	s := c.Sanitize()
	if len(s.TranslationConfig) != 1 || s.TranslationConfig["model"] != "m" {
		t.Fatalf("%+v", s.TranslationConfig)
	}
}

func TestNewProjectID(t *testing.T) {
	a, b := NewProjectID(), NewProjectID()
	if a == b || len(a) != 36 || a[14] != '4' {
		t.Fatalf("%s %s", a, b)
	}
}
