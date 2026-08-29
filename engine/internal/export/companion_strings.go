package export

import "strings"

// The archive is written in the reader's language.
//
// It was Korean for everyone, which meant an English writer got a Korean
// wrapper around their own English conversations (#45). The transcript itself
// is theirs and is copied through untouched; only the labels around it are ours.
type companionStrings struct {
	title          string
	exportedAt     string
	noticeLine1    string
	noticeLine2    string
	nothingLeft    string
	untitled       string
	conversation   string
	emptyMessage   string
	remembered     string
	memoryTruncate string
	speakerUser    string
	speakerAgent   string
}

func companionStringsFor(language string) companionStrings {
	switch {
	case strings.HasPrefix(language, "en"):
		return companionStrings{
			title:       "Linetta companion archive",
			exportedAt:  "Exported",
			noticeLine1: "Linetta is a writing tool now, and the built-in AI companion is removed in the next major version.",
			noticeLine2: "This file keeps the conversations before that happens.",
			nothingLeft: "No companion conversations or memories remain.",
			untitled:    "Untitled",

			conversation: "Conversation",
			emptyMessage: "(no content)",
			remembered:   "Remembered",
			memoryTruncate: "> At most %d memories are included here. The full record stays at " +
				"`memory/experiences.jsonl` under `%s`, and that file is not deleted when the companion is.\n\n",
			speakerUser:  "Me",
			speakerAgent: "Companion",
		}
	case strings.HasPrefix(language, "ja"):
		return companionStrings{
			title:       "リネッタ コンパニオン記録",
			exportedAt:  "書き出した時刻",
			noticeLine1: "リネッタは純粋な執筆ツールになり、内蔵AIコンパニオンは次のメジャーバージョンで削除されます。",
			noticeLine2: "このファイルはその前に会話記録を残しておくためのものです。",
			nothingLeft: "残っているコンパニオンの会話や記憶はありません。",
			untitled:    "無題",

			conversation: "会話",
			emptyMessage: "(内容なし)",
			remembered:   "記憶したこと",
			memoryTruncate: "> 記憶は最大%d件までです。全記録は `%s` の下の " +
				"`memory/experiences.jsonl` にそのまま残り、コンパニオンが削除されても消えません。\n\n",
			speakerUser:  "私",
			speakerAgent: "コンパニオン",
		}
	default:
		return companionStrings{
			title:       "리네타 컴패니언 기록",
			exportedAt:  "내보낸 시각",
			noticeLine1: "리네타는 순수 창작 도구로 전환했고, 내장 AI 컴패니언은 다음 주요 버전에서 제거됩니다.",
			noticeLine2: "이 파일은 그 전에 대화 기록을 남겨 두기 위한 것입니다.",
			nothingLeft: "남아 있는 컴패니언 대화나 기억이 없습니다.",
			untitled:    "제목 없음",

			conversation: "대화",
			emptyMessage: "(내용 없음)",
			remembered:   "기억한 것",
			memoryTruncate: "> 기억은 최대 %d개까지만 담깁니다. 전체 기록은 `%s` 아래 " +
				"`memory/experiences.jsonl`에 그대로 남아 있으며, 이 파일은 컴패니언이 제거돼도 지워지지 않습니다.\n\n",
			speakerUser:  "나",
			speakerAgent: "컴패니언",
		}
	}
}
