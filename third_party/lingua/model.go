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

// 本文件由 CodeI18n 修改：删除了仅用于训练/写出语言模型的 trainingDataLanguageModel
// 及其相关函数（computeAbsoluteFrequencies、computeRelativeFrequencies），只保留检测所需部分。

import (
	"fmt"
)

type testDataLanguageModel struct {
	ngrams [][]ngram
}

func newTestDataLanguageModel(words []string, ngramLength int) testDataLanguageModel {
	if ngramLength > maxNgramLength {
		panic(fmt.Sprintf("ngram length %v is greater than %v", ngramLength, maxNgramLength))
	}
	ngrams := make(map[ngram]struct{})

	for _, word := range words {
		chars := []rune(word)
		charsCount := len(chars)

		if charsCount >= ngramLength {
			for i := 0; i <= charsCount-ngramLength; i++ {
				slice := string(chars[i : i+ngramLength])
				ngrams[newNgram(slice)] = struct{}{}
			}
		}
	}

	lowerOrderNgrams := make([][]ngram, len(ngrams))
	i := 0
	for n := range ngrams {
		lowerOrderNgrams[i] = n.rangeOfLowerOrderNgrams()
		i++
	}

	return testDataLanguageModel{ngrams: lowerOrderNgrams}
}
