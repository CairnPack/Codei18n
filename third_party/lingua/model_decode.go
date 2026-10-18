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

// 本文件由 CodeI18n 新增：用一个极简的手写 protobuf 解码器替代上游对
// google.golang.org/protobuf 与生成代码 serialization/model.pb.go 的依赖。
// 模型文件格式见上游 serialization/model.proto：
//
//	message SerializableLanguageModel {
//	  SerializableLanguage language = 1;      // varint
//	  uint32 ngram_length = 2;                // varint
//	  uint32 total_ngrams = 3;                // varint
//	  repeated SerializableNgramSet ngram_sets = 4; // length-delimited
//	}
//	message SerializableNgramSet {
//	  double probability = 1;                 // fixed64
//	  repeated string ngrams = 2;             // length-delimited
//	}
//
// 解码器对未知字段按线型跳过，与标准 protobuf 语义一致。

import (
	"encoding/binary"
	"errors"
	"math"
)

// protobuf 线型（wire type）。
const (
	wireVarint  = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

var errMalformedModel = errors.New("lingua: 语言模型数据格式错误")

// protoReader 顺序读取 protobuf 字节流。
type protoReader struct {
	buf []byte
	pos int
}

func (r *protoReader) done() bool { return r.pos >= len(r.buf) }

func (r *protoReader) varint() (uint64, error) {
	v, n := binary.Uvarint(r.buf[r.pos:])
	if n <= 0 {
		return 0, errMalformedModel
	}
	r.pos += n
	return v, nil
}

func (r *protoReader) fixed64() (uint64, error) {
	if len(r.buf)-r.pos < 8 {
		return 0, errMalformedModel
	}
	v := binary.LittleEndian.Uint64(r.buf[r.pos:])
	r.pos += 8
	return v, nil
}

func (r *protoReader) bytes() ([]byte, error) {
	l, err := r.varint()
	if err != nil {
		return nil, err
	}
	if l > uint64(len(r.buf)-r.pos) {
		return nil, errMalformedModel
	}
	b := r.buf[r.pos : r.pos+int(l)]
	r.pos += int(l)
	return b, nil
}

// tag 读取字段号与线型。
func (r *protoReader) tag() (field uint64, wire uint64, err error) {
	t, err := r.varint()
	if err != nil {
		return 0, 0, err
	}
	return t >> 3, t & 7, nil
}

// skip 跳过一个不关心的字段。
func (r *protoReader) skip(wire uint64) error {
	switch wire {
	case wireVarint:
		_, err := r.varint()
		return err
	case wireFixed64:
		_, err := r.fixed64()
		return err
	case wireBytes:
		_, err := r.bytes()
		return err
	case wireFixed32:
		if len(r.buf)-r.pos < 4 {
			return errMalformedModel
		}
		r.pos += 4
		return nil
	default:
		return errMalformedModel
	}
}

// decodeLanguageModel 把序列化的 SerializableLanguageModel 解码为 ngram → 概率 映射。
func decodeLanguageModel(data []byte) (map[string]float64, error) {
	r := protoReader{buf: data}
	var model map[string]float64
	for !r.done() {
		field, wire, err := r.tag()
		if err != nil {
			return nil, err
		}
		switch {
		case field == 3 && wire == wireVarint:
			total, err := r.varint()
			if err != nil {
				return nil, err
			}
			if model == nil && total < 1<<24 {
				model = make(map[string]float64, int(total))
			}
		case field == 4 && wire == wireBytes:
			set, err := r.bytes()
			if err != nil {
				return nil, err
			}
			if model == nil {
				model = make(map[string]float64)
			}
			if err := decodeNgramSet(set, model); err != nil {
				return nil, err
			}
		default:
			if err := r.skip(wire); err != nil {
				return nil, err
			}
		}
	}
	if model == nil {
		model = make(map[string]float64)
	}
	return model, nil
}

// decodeNgramSet 解码一个 SerializableNgramSet 并写入 model。
// protobuf 允许字段以任意顺序出现，因此先收集 ngram 再统一赋概率。
func decodeNgramSet(data []byte, model map[string]float64) error {
	r := protoReader{buf: data}
	var probability float64
	var ngrams []string
	for !r.done() {
		field, wire, err := r.tag()
		if err != nil {
			return err
		}
		switch {
		case field == 1 && wire == wireFixed64:
			bits, err := r.fixed64()
			if err != nil {
				return err
			}
			probability = math.Float64frombits(bits)
		case field == 2 && wire == wireBytes:
			b, err := r.bytes()
			if err != nil {
				return err
			}
			ngrams = append(ngrams, string(b))
		default:
			if err := r.skip(wire); err != nil {
				return err
			}
		}
	}
	for _, n := range ngrams {
		model[n] = probability
	}
	return nil
}
