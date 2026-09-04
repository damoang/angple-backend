package contentkind

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		// 이미지만 있는 댓글이 Empty 로 오분류돼 "빈 댓글"로 보이던 핵심 회귀(#13095 후속).
		{"image only", `<p><img src="https://cdn.damoang.net/data/editor/2609/079296c.webp" alt="첨부 이미지"></p>`, Image},
		{"image multiple", `<p><img src="/data/editor/a.jpeg"><img src="/data/editor/b.jpeg"></p>`, Image},
		{"text with image is text", `<p>이것 보세요 <img src="/data/editor/x.png"></p>`, Text},
		{"plain text", `<p>안녕하세요 반갑습니다</p>`, Text},

		// 이모티콘 두 형식(우선순위: 이미지보다 앞).
		{"emoticon code", `{emo:hello}`, Emoticon},
		{"emoticon img path", `<p><img src="https://cdn.damoang.net/emoticons/smile.png"></p>`, Emoticon},

		{"video tag", `<p><video src="x.mp4"></video></p>`, Video},
		{"iframe", `<p><iframe src="https://youtube.com/embed/x"></iframe></p>`, Video},
		{"link only", `<p><a href="https://example.com">링크</a></p>`, Text}, // 앵커 안 텍스트가 남으면 Text
		{"link no text", `<p><a href="https://example.com"></a></p>`, Link},

		// 빈 값 판정: 태그만/공백만/점자공백(U+2800, 빈댓글 기능)만.
		{"empty", ``, Empty},
		{"tags only", `<p></p>`, Empty},
		{"whitespace only", `<p>   </p>`, Empty},
		{"braille blank only", "<p>⠀⠀⠀</p>", Empty},
		{"nbsp only", `<p>&nbsp;&nbsp;</p>`, Empty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.content); got != tc.want {
				t.Errorf("Classify(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}
