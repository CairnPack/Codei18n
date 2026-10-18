/*
 * Copyright © 2021-present Peter M. Stahl pemistahl@gmail.com
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either expressed or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package lingua

// 本文件由 CodeI18n 新增。

import "sort"

// AvailableLanguages 返回本裁剪版实际内嵌了语言模型的语言（按枚举值升序）。
//
// 上游的 Language 枚举仍保留全部 75 种语言，但本副本只内嵌了少数语言的模型；
// 对没有模型的语言构建检测器不会报错，只是统计模型部分永远查不到 n-gram，
// 因此调用方应只使用本函数返回的语言。
func AvailableLanguages() []Language {
	entries, err := languageModels.ReadDir("language-models")
	if err != nil {
		return nil
	}
	var languages []Language
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		language := GetLanguageFromIsoCode639_1(GetIsoCode639_1FromValue(entry.Name()))
		if language != Unknown {
			languages = append(languages, language)
		}
	}
	sort.Slice(languages, func(i, j int) bool { return languages[i] < languages[j] })
	return languages
}
