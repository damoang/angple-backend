package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractKeywords_Basic(t *testing.T) {
	keywords := extractKeywords("Go 프로그래밍 입문", "Go 언어를 배우는 방법을 알아봅니다. 프로그래밍 기초부터 시작합니다.")

	assert.NotNil(t, keywords)
	// "프로그래밍" appears in title (3x weight) + content
	assert.Contains(t, keywords, "프로그래밍")
}

func TestExtractKeywords_English(t *testing.T) {
	keywords := extractKeywords("Golang Tutorial", "Learn golang programming with examples. Golang is great for backend.")

	assert.NotNil(t, keywords)
	assert.Contains(t, keywords, "golang")
}

func TestExtractKeywords_Empty(t *testing.T) {
	keywords := extractKeywords("", "")
	assert.Nil(t, keywords)
}

func TestExtractKeywords_StopWordsFiltered(t *testing.T) {
	keywords := extractKeywords("the and for", "the and for but not you all")
	// All stop words, should return nil or empty
	assert.Nil(t, keywords)
}

func TestExtractKeywords_HTMLStripped(t *testing.T) {
	keywords := extractKeywords("테스트", "<p>리액트 <b>컴포넌트</b> 만들기</p>")

	assert.NotNil(t, keywords)
	assert.Contains(t, keywords, "리액트")
	assert.Contains(t, keywords, "컴포넌트")
}

func TestExtractKeywords_MaxTopics(t *testing.T) {
	// Generate content with many unique keywords
	content := "알파 베타 감마 델타 엡실론 제타 에타 쎄타 이오타 카파 람다 시그마 오메가 파이널"
	keywords := extractKeywords("테스트 제목", content)

	// Should cap at 10 topics
	assert.LessOrEqual(t, len(keywords), 10)
}

func TestIsStopWord(t *testing.T) {
	assert.True(t, isStopWord("the"))
	assert.True(t, isStopWord("그리고"))
	assert.True(t, isStopWord("입니다"))
	assert.False(t, isStopWord("golang"))
	assert.False(t, isStopWord("프로그래밍"))
}

func TestContainsStr(t *testing.T) {
	slice := []string{"a", "b", "c"}
	assert.True(t, containsStr(slice, "b"))
	assert.False(t, containsStr(slice, "d"))
	assert.False(t, containsStr(nil, "a"))
}
