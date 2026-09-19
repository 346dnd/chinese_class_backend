// Package grading 提供文本批改工具函数。
package grading

import (
	"strings"
	"unicode"
)

// GradeResult 答案批改结果。
type GradeResult struct {
	// IsPassed 答案是否合格。
	IsPassed bool
	// Feedback AI 反馈文本（可选，简单匹配时为空）。
	Feedback string
	// ReferenceAnswer 参考答案（用于 get-answer 或完成时返回）。
	ReferenceAnswer string
}

// GradeTextSimple 简单文本匹配批改。
//
// 比对策略：
//  1. 去除首尾空白、统一全角/半角空格
//  2. 忽略大小写（对英文）
//  3. 中英文混排时，去除所有标点符号后比对
//
// 注意：这是简化版批改，仅适合客观题；主观题应通过 AIService 批改。
func GradeTextSimple(input, reference string) GradeResult {
	if NormalizeText(input) == NormalizeText(reference) {
		return GradeResult{
			IsPassed:        true,
			ReferenceAnswer: reference,
		}
	}
	return GradeResult{
		IsPassed:        false,
		ReferenceAnswer: reference,
	}
}

// GradeZhaoZhouQiaoText 赵州桥填空题批改。
//
// 考虑三年级学生的表达水平，允许在标准答案前加常见程度修饰词（很/非常/十分/特别），
// 视为正确答案；判定通过后参考答案仍返回标准答案本身（不展示学生变体，如"很美观"）。
// 例如参考"美观"时，"很美观""非常美观"等均判为正确。
func GradeZhaoZhouQiaoText(input, reference string) GradeResult {
	if GradeTextSimple(input, reference).IsPassed {
		return GradeResult{IsPassed: true, ReferenceAnswer: reference}
	}
	for _, m := range []string{"很", "非常", "十分", "特别"} {
		if GradeTextSimple(input, m+reference).IsPassed {
			return GradeResult{IsPassed: true, ReferenceAnswer: reference}
		}
	}
	return GradeResult{IsPassed: false, ReferenceAnswer: reference}
}

// NormalizeText 归一化文本：去首尾空白、转小写、去标点。
func NormalizeText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	// 全角空格转半角
	s = strings.ReplaceAll(s, "\u3000", " ")
	// 去所有标点（中英文）
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPunct(r) || unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}